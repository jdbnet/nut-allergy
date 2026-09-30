package webhooks

func teamsTitleColor(event string) string {
	switch event {
	case "on_battery":
		return "Attention"
	case "on_mains":
		return "Good"
	case "low_battery":
		return "Warning"
	default:
		return "Default"
	}
}

func discordColor(event string) int {
	switch event {
	case "on_battery":
		return 0xC50F1F
	case "on_mains":
		return 0x107C10
	case "low_battery":
		return 0xFF8C00
	default:
		return 0x5B5FC7
	}
}

func gotifyPriority(event string) int {
	switch event {
	case "on_battery", "low_battery":
		return 8
	case "on_mains":
		return 4
	default:
		return 5
	}
}

func ntfyPriority(event string) int {
	switch event {
	case "low_battery":
		return 5
	case "on_battery":
		return 4
	case "on_mains":
		return 3
	default:
		return 3
	}
}

func ntfyTags(event string) []string {
	switch event {
	case "on_battery":
		return []string{"warning", "battery", "zap"}
	case "on_mains":
		return []string{"white_check_mark", "electric_plug"}
	case "low_battery":
		return []string{"warning", "battery"}
	default:
		return []string{"bell"}
	}
}
