package ecoflowdelta3

import "fmt"

type Topics struct {
	Get      string
	GetReply string
	Set      string
	SetReply string
	Data     string
}

func BuildTopics(userID string, deviceSN string) Topics {
	return Topics{
		Get:      fmt.Sprintf("/app/%s/%s/thing/property/get", userID, deviceSN),
		GetReply: fmt.Sprintf("/app/%s/%s/thing/property/get_reply", userID, deviceSN),
		Set:      fmt.Sprintf("/app/%s/%s/thing/property/set", userID, deviceSN),
		SetReply: fmt.Sprintf("/app/%s/%s/thing/property/set_reply", userID, deviceSN),
		Data:     fmt.Sprintf("/app/device/property/%s", deviceSN),
	}
}
