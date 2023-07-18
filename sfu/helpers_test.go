package sfu

import (
	"github.com/ducksouplab/ducksoup/types"
)

func newJoinPayload(origin, interactionName, userId, namespace string, size int) types.JoinPayload {
	return types.JoinPayload{
		Origin:          origin,
		InteractionName: interactionName,
		UserId:          userId,
		Namespace:       namespace,
		Size:            size,
	}
}
