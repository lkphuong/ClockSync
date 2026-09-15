package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	_ "time/tzdata"
)

const (
	timeAPIURL         = "https://timeapi.io/api/time/current/zone?timeZone=Asia/Ho_Chi_Minh"
	timeAPITimeLayout  = "2006-01-02T15:04:05"
	systemLocationName = "Asia/Ho_Chi_Minh"

	syncInterval    = 5 * time.Minute
	maxAllowedDrift = 10 * time.Minute
)

type timeAPIResponse struct {
	DateTime string `json:"dateTime"`
}

type logEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// logStructured ghi log dạng JSON ra stdout, kèm level và lỗi (nếu có).
func logStructured(level, message string, err error) {
	entry := logEntry{
		Time:    time.Now().Format(time.RFC3339),
		Level:   level,
		Message: message,
	}
	if err != nil {
		entry.Error = err.Error()
	}

	data, marshalErr := json.Marshal(entry)
	if marshalErr != nil {
		return
	}
	fmt.Println(string(data))
}

func logInfo(message string) {
	logStructured("INFO", message, nil)
}

func logError(message string, err error) {
	logStructured("ERROR", message, err)
}

func getTimeFromTimeAPI() (time.Time, error) {
	resp, err := http.Get(timeAPIURL)
	if err != nil {
		return time.Time{}, fmt.Errorf("không gọi được timeapi.io: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("timeapi.io trả về mã lỗi %d", resp.StatusCode)
	}

	var payload timeAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return time.Time{}, fmt.Errorf("không parse được phản hồi từ timeapi.io: %w", err)
	}

	loc, err := time.LoadLocation(systemLocationName)
	if err != nil {
		return time.Time{}, fmt.Errorf("không tải được múi giờ %s: %w", systemLocationName, err)
	}

	return time.ParseInLocation(timeAPITimeLayout, payload.DateTime, loc)
}

func setSystemDateTime(t time.Time) error {
	psCommand := fmt.Sprintf(
		"Set-Date -Date (Get-Date -Year %d -Month %d -Day %d -Hour %d -Minute %d -Second %d)",
		t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second(),
	)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("không đặt được ngày giờ hệ thống qua PowerShell: %w (output: %s)", err, string(output))
	}
	return nil
}

// timeDrift trả về độ lệch (giá trị tuyệt đối) giữa giờ hệ thống hiện tại và giờ chuẩn.
func timeDrift(referenceTime time.Time) time.Duration {
	drift := referenceTime.Sub(time.Now().In(referenceTime.Location()))
	if drift < 0 {
		drift = -drift
	}
	return drift
}

func updateSystemTime() error {
	referenceTime, err := getTimeFromTimeAPI()
	if err != nil {
		return err
	}

	// Chỉ chỉnh giờ hệ thống khi lệch quá ngưỡng cho phép, tránh chỉnh liên tục không cần thiết.
	drift := timeDrift(referenceTime)
	if drift <= maxAllowedDrift {
		logInfo(fmt.Sprintf("giờ hệ thống lệch %s, trong ngưỡng cho phép, bỏ qua cập nhật", drift))
		return nil
	}

	logInfo(fmt.Sprintf("giờ hệ thống lệch %s, vượt ngưỡng %s, tiến hành cập nhật", drift, maxAllowedDrift))

	if err := setSystemDateTime(referenceTime); err != nil {
		return err
	}

	logInfo(fmt.Sprintf("cập nhật giờ hệ thống thành công theo giờ %s", referenceTime.Format(time.RFC3339)))
	return nil
}

func main() {
	if err := updateSystemTime(); err != nil {
		logError("đồng bộ giờ lần đầu thất bại", err)
	}

	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := updateSystemTime(); err != nil {
			logError("đồng bộ giờ định kỳ thất bại", err)
		}
	}
}
