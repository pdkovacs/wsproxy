package test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"nhooyr.io/websocket"

	wsgw "websocket-gateway/internal"
)

type baseTestSuite struct {
	suite.Suite
	wsgwServer string
	mockApp    *mockApplication
	wsGateway  *wsgw.Server
	nextConnId wsgw.ConnectionID
	ctx        context.Context
}

func NewBaseTestSuite(ctx context.Context) *baseTestSuite {
	return &baseTestSuite{
		ctx: ctx,
	}
}

var connectionIdGenerator = func(getNextId func() wsgw.ConnectionID) func() wsgw.ConnectionID {
	return func() wsgw.ConnectionID {
		return getNextId()
	}
}

func (s *baseTestSuite) startMockApp() {
	s.mockApp = newMockApp(func() string {
		return fmt.Sprintf("http://%s", s.wsgwServer)
	})
	mockAppStartErr := s.mockApp.start()
	if mockAppStartErr != nil {
		panic(mockAppStartErr)
	}
}

func (s *baseTestSuite) SetupSuite() {
	logger := zerolog.Ctx(s.ctx).With().Str("method", "SetupSuite").Logger()
	logger.Info().Msg("BEGIN")

	s.startMockApp()

	server := wsgw.NewServer(
		s.ctx,
		wsgw.Config{
			ServerHost:          "localhost",
			ServerPort:          0,
			AppBaseUrl:          fmt.Sprintf("http://%s", s.mockApp.listener.Addr().String()),
			LoadBalancerAddress: "",
		},
		connectionIdGenerator(func() wsgw.ConnectionID {
			return s.nextConnId
		}),
	)
	s.wsGateway = server

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := server.SetupAndStart(func(port int, _ func()) {
			fmt.Fprint(os.Stderr, "WsGateway is ready!")
			s.wsgwServer = fmt.Sprintf("localhost:%d", port)
			wg.Done()
		})
		logger.Warn().Err(err).Msg("error during server start")
	}()
	wg.Wait()
}

func (s *baseTestSuite) TearDownSuite() {
	if s.mockApp != nil {
		s.mockApp.stop()
	}
	if s.wsGateway != nil {
		s.wsGateway.Stop()
	}
}

func (s *baseTestSuite) getCall(callIndex int) *mock.Call {
	return &s.mockApp.mockMock.Calls[callIndex]
}

func (s *baseTestSuite) assertArguments(call *mock.Call, objects ...interface{}) {
	call.Arguments.Assert(s.T(), objects...)
}

func (s *baseTestSuite) connectToWsgw(ctx context.Context, options *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
	return websocket.Dial(ctx, fmt.Sprintf("ws://%s%s", s.wsgwServer, wsgw.ConnectPath), options)
}

var defaultDialOptions = &websocket.DialOptions{
	HTTPHeader: http.Header{
		"Authorization": []string{"some credentials"},
	},
}

func toWsMessage(content string) messageJSON {
	return messageJSON{"message": content}
}
