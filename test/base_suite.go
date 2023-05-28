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
	"websocket-gateway/internal/logging"
)

type baseTestSuite struct {
	suite.Suite
	wsgwServer string
	mockApp    *mockApplication
	wsGateway  *wsgw.Server
	logger     zerolog.Logger
	nextConnId wsgw.ConnectionID
}

func NewBaseTestSuite(logger zerolog.Logger) *baseTestSuite {
	return &baseTestSuite{
		logger: logger,
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
	logger := logging.CreateMethodLogger(s.logger, "SetupSuite")
	logger.Info().Msg("BEGIN")

	s.startMockApp()

	server := wsgw.NewServer(
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
	go server.SetupAndStart(func(port int, _ func()) {
		fmt.Fprint(os.Stderr, "WsGateway is ready!")
		s.wsgwServer = fmt.Sprintf("localhost:%d", port)
		wg.Done()
	})
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
	call.Arguments.Assert(s.T(), wsgw.ConnectionIDHeaderKey, string(s.nextConnId))
}

func (s *baseTestSuite) connectToWsgw(ctx context.Context, options *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
	return websocket.Dial(ctx, fmt.Sprintf("ws://%s%s", s.wsgwServer, wsgw.ConnectPath), options)
}

var defaultDialOptions = &websocket.DialOptions{
	HTTPHeader: http.Header{
		"Authorization": []string{"some credentials"},
	},
}
