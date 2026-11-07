// Package httpx contains shared HTTP response and error helpers.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// AppError is an error carrying an HTTP status code.
type AppError struct {
	Status  int
	Message string
	Details any
}

func (e *AppError) Error() string { return e.Message }

// NewError builds an AppError.
func NewError(status int, message string) *AppError {
	return &AppError{Status: status, Message: message}
}

// StatusOf is the HTTP status of a domain fault: the one place where business kinds meet HTTP.
func StatusOf(k domain.FaultKind) int {
	switch k {
	case domain.FaultInvalid:
		return fiber.StatusBadRequest
	case domain.FaultUnauthorized:
		return fiber.StatusUnauthorized
	case domain.FaultForbidden:
		return fiber.StatusForbidden
	case domain.FaultNotFound:
		return fiber.StatusNotFound
	case domain.FaultConflict:
		return fiber.StatusConflict
	case domain.FaultUnprocessable:
		return fiber.StatusUnprocessableEntity
	case domain.FaultLocked:
		return fiber.StatusLocked
	case domain.FaultRateLimited:
		return fiber.StatusTooManyRequests
	case domain.FaultUnavailable:
		return fiber.StatusBadGateway
	}
	return fiber.StatusInternalServerError
}

// OK sends {success:true,data}.
func OK(c *fiber.Ctx, data any) error {
	return c.JSON(fiber.Map{"success": true, "data": data})
}

// OKMessage sends {success:true,data,message}.
func OKMessage(c *fiber.Ctx, data any, message string) error {
	return c.JSON(fiber.Map{"success": true, "data": data, "message": message})
}

// Created sends a 201 {success:true,data}.
func Created(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "data": data})
}

// Fail sends {success:false,message} with the given status.
func Fail(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(fiber.Map{"success": false, "message": message})
}

var validate = validator.New(validator.WithRequiredStructEnabled())

// BindAndValidate parses the JSON body into dst and validates it.
func BindAndValidate(c *fiber.Ctx, dst any) error {
	if err := c.BodyParser(dst); err != nil {
		return &AppError{Status: fiber.StatusBadRequest, Message: "Dữ liệu không hợp lệ"}
	}
	if err := validate.Struct(dst); err != nil {
		details := map[string]string{}
		var verrs validator.ValidationErrors
		if errors.As(err, &verrs) {
			for _, fe := range verrs {
				details[fe.Field()] = fe.Tag()
			}
		}
		return &AppError{Status: fiber.StatusBadRequest, Message: "Dữ liệu không hợp lệ", Details: details}
	}
	return nil
}

// internalErrorMessage is what a client sees instead of the text of an infrastructure failure.
const internalErrorMessage = "Lỗi hệ thống. Vui lòng thử lại sau."

// ErrorHandler is the global Fiber error handler (mirrors the former Express one). It shows the text of every
// unexpected error, which helps while developing; production uses NewErrorHandler(false).
func ErrorHandler(c *fiber.Ctx, err error) error { return handleError(c, err, true) }

// NewErrorHandler builds the global Fiber error handler. With expose=false the text of an infrastructure failure
// (database driver, network, timeout) is replaced by a generic sentence and the real error goes to the log with the
// request id, so a client never reads a host name or a query. A plain error written for the user ("KOC chưa có
// email") is still shown as it is.
func NewErrorHandler(expose bool) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error { return handleError(c, err, expose) }
}

// isInfrastructureError reports whether err comes from the database driver, the network or a deadline. The driver is
// recognised by its type name so that httpx does not import it.
func isInfrastructureError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	for e := err; e != nil; e = errors.Unwrap(e) {
		name := fmt.Sprintf("%T", e)
		if strings.Contains(name, "mongo") || strings.Contains(name, "topology") || strings.Contains(name, "driver.") {
			return true
		}
	}
	return false
}

func handleError(c *fiber.Ctx, err error, expose bool) error {
	status := fiber.StatusInternalServerError
	message := "Internal server error"
	var details any

	var appErr *AppError
	var fault *domain.Fault
	var fiberErr *fiber.Error
	switch {
	case errors.As(err, &appErr):
		status, message, details = appErr.Status, appErr.Message, appErr.Details
	case errors.As(err, &fault):
		status, message = StatusOf(fault.Kind), fault.Message
	case errors.As(err, &fiberErr):
		status, message = fiberErr.Code, fiberErr.Message
		if status == fiber.StatusNotFound {
			message = fmt.Sprintf("Route not found: %s %s", c.Method(), c.OriginalURL())
		}
	case err != nil:
		message = err.Error()
		if !expose && isInfrastructureError(err) {
			log.Printf("internal error [%s] %s %s: %v", c.GetRespHeader(fiber.HeaderXRequestID), c.Method(), c.Path(), err)
			message = internalErrorMessage
		}
	}
	body := fiber.Map{"success": false, "message": message}
	if details != nil {
		body["details"] = details
	}
	return c.Status(status).JSON(body)
}
