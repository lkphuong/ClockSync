func enableWindowsAutoTime() error {
	// Enable Windows Time service
	cmd := exec.Command("cmd", "/C", `sc config w32time start= auto`)
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("cmd", "/C", `net start w32time`)
	_ = cmd.Run() // đã chạy thì bỏ qua lỗi

	// Configure NTP
	cmd = exec.Command(
		"cmd", "/C",
		`w32tm /config /syncfromflags:manual /manualpeerlist:"time.windows.com" /update`,
	)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Sync now
	cmd = exec.Command("cmd", "/C", `w32tm /resync`)
	return cmd.Run()
}