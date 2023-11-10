package wsproxy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	"wsproxy/internal/logging"

	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
)

type connection struct {
	fromClient chan string
	fromApp    chan string
	connClosed chan websocket.CloseError
	closeSlow  func()
	id         ConnectionID
}

type wsConnections struct {
	connectionMessageBuffer int

	// publishLimiter controls the rate limit applied to the publish endpoint.
	//
	// Defaults to one publish every 100ms with a burst of 8.
	publishLimiter *rate.Limiter

	connectionsMu sync.Mutex
	wsMap         map[ConnectionID]*connection

	logger zerolog.Logger
}

var errConnectionNotFound = errors.New("connection not found")

func newWsConnections() *wsConnections {
	ns := &wsConnections{
		connectionMessageBuffer: 16,
		wsMap:                   make(map[ConnectionID]*connection),
		publishLimiter:          rate.NewLimiter(rate.Every(time.Millisecond*100), 8),
		logger:                  logging.Get().With().Str("unit", "notification-server").Logger(),
	}

	return ns
}

type wsIO interface {
	Close() error
	Write(ctx context.Context, msg string) error
	Read(ctx context.Context) (string, error)
}

type onMgsReceivedFunc func(c context.Context, msg string) error

func (wsconn *wsConnections) processMessages(
	ctx context.Context,
	parentLogger zerolog.Logger,
	wsIo wsIO,
	onMessageFromClient onMgsReceivedFunc,
) error {
	logger := parentLogger.With().Str("method", "processMessages").Logger()
	conn := &connection{
		fromClient: make(chan string),
		fromApp:    make(chan string, wsconn.connectionMessageBuffer),
		connClosed: make(chan websocket.CloseError),
		closeSlow: func() {
			wsIo.Close()
		},
	}

	wsconn.addConnection(conn)
	defer wsconn.deleteConnection(conn)

	go func() {
		for {
			msgRead, errRead := wsIo.Read(ctx)
			if errRead != nil {
				logger.Debug().Msgf("Read error: %v", errRead)
				var closeError websocket.CloseError
				if errors.As(errRead, &closeError) {
					logger.Debug().Err(errRead).Msg("WS connection closing...")
					conn.connClosed <- closeError
					return
				}
				logger.Error().Err(errRead).Msg("WS connection not closing")
				select {
				case conn.fromClient <- "asdfasdf":
				default:
					go wsIo.Close()
				}
				return
			}
			conn.fromClient <- msgRead
		}
	}()

	for {
		logger.Debug().Msg("about to enter select...")
		select {
		case msg := <-conn.fromApp:
			logger.Debug().Msg("select: msg from backend")
			err := writeTimeout(ctx, time.Second*5, wsIo, msg)
			if err != nil {
				return err
			}
		case msg := <-conn.fromClient:
			sendToAppErr := onMessageFromClient(ctx, msg)
			if sendToAppErr != nil {
				conn.fromApp <- sendToAppErr.Error()
			}
		case closeError := <-conn.connClosed:
			logger.Debug().Err(closeError).Msg("WS connection closing...")
			if closeError.Code == websocket.StatusNormalClosure {
				return nil
			}
			logger.Error().Err(closeError).Msg("socket closed abnormaly")
			return fmt.Errorf("socket closed abnormaly: %w", closeError)
		case <-ctx.Done():
			logger.Debug().Msg("select: context is done")
			return ctx.Err()
		}
		logger.Debug().Msg("exited select")
	}
}

// addConnection registers a subscriber.
func (wsconn *wsConnections) addConnection(conn *connection) {
	wsconn.connectionsMu.Lock()
	defer wsconn.connectionsMu.Unlock()
	wsconn.wsMap[conn.id] = conn
}

// deleteConnection deletes the given subscriber.
func (wsconn *wsConnections) deleteConnection(conn *connection) {
	wsconn.connectionsMu.Lock()
	defer wsconn.connectionsMu.Unlock()
	defer delete(wsconn.wsMap, conn.id)
}

// It never blocks and so messages to slow subscribers
// are dropped.
func (wsconn *wsConnections) push(ctx context.Context, msg string, connId ConnectionID) error {
	wsconn.connectionsMu.Lock()
	defer wsconn.connectionsMu.Unlock()

	wsconn.publishLimiter.Wait(ctx)

	conn, connNotFoundErr := wsconn.getConnection(connId)
	if connNotFoundErr != nil {
		return connNotFoundErr
	}

	conn.fromApp <- string(msg)

	return nil
}

func (wsconn *wsConnections) getConnection(connId ConnectionID) (*connection, error) {
	wsconn.connectionsMu.Lock()
	defer wsconn.connectionsMu.Unlock()
	conn, ok := wsconn.wsMap[connId]
	if !ok {
		return nil, errConnectionNotFound
	}
	return conn, nil
}

func writeTimeout(ctx context.Context, timeout time.Duration, sIo wsIO, msg string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return sIo.Write(ctx, msg)
}
