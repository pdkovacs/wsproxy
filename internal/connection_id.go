package wsgw

import (
	"context"

	"github.com/rs/xid"
	"github.com/rs/zerolog"
)

type ConnectionID string

func CreateID(ctx context.Context) ConnectionID {
	logger := zerolog.Ctx(ctx)
	connectionId := xid.New().String()
	logger.Debug().Str("connectionId", connectionId).Msg("connectionId created")
	return ConnectionID(connectionId)
}
