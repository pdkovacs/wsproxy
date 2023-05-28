package wsgw

import (
	"github.com/rs/xid"
	"github.com/rs/zerolog"
)

type ConnectionID string

func CreateID(logger zerolog.Logger) ConnectionID {
	connectionId := xid.New().String()
	logger.Debug().Str("connectionId", connectionId).Msg("connectionId created")
	return ConnectionID(connectionId)
}
