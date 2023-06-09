package types

import "fmt"

type JoinPayload struct {
	InteractionName string `json:"interactionName"`
	UserId          string `json:"userId"`
	Duration        int    `json:"duration"`
	// optional
	Namespace     string `json:"namespace"`
	VideoFormat   string `json:"videoFormat"`
	RecordingMode string `json:"recordingMode"`
	Size          int    `json:"size"`
	AudioFx       string `json:"audioFx"`
	VideoFx       string `json:"videoFx"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Framerate     int    `json:"framerate"`
	GPU           bool   `json:"gpu"`
	Overlay       bool   `json:"overlay"`
	// Not from JSON
	Origin string
}

func (join JoinPayload) DataFolder() string {
	return fmt.Sprintf("data/%v/%v", join.Namespace, join.InteractionName)
}

type TrackWriter interface {
	ID() string
	Write(buf []byte) error
}
