package service

import "time"

// nextClockTime is the next moment it is atMinute minutes after midnight in Vietnam: today's when that time has not come yet,
// tomorrow's otherwise (also at exactly that time, so a block never waits for nothing).
func nextClockTime(now time.Time, atMinute int) time.Time {
	vn := now.In(vnZone)
	at := time.Date(vn.Year(), vn.Month(), vn.Day(), atMinute/60, atMinute%60, 0, 0, vnZone)
	if vn.Before(at) {
		return at
	}
	return at.AddDate(0, 0, 1)
}
