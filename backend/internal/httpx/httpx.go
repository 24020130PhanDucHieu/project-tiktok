// Package httpx contains shared HTTP response and error helpers.
package httpx

import (
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
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

// ErrorHandler is the global Fiber error handler (mirrors the former Express one).
func ErrorHandler(c *fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	message := "Internal server error"
	var details any

	var appErr *AppError
	var fiberErr *fiber.Error
	switch {
	case errors.As(err, &appErr):
		status, message, details = appErr.Status, appErr.Message, appErr.Details
	case errors.As(err, &fiberErr):
		status, message = fiberErr.Code, fiberErr.Message
		if status == fiber.StatusNotFound {
			message = fmt.Sprintf("Route not found: %s %s", c.Method(), c.OriginalURL())
		}
	case err != nil:
		message = err.Error()
	}
	body := fiber.Map{"success": false, "message": message}
	if details != nil {
		body["details"] = details
	}
	return c.Status(status).JSON(body)
}
