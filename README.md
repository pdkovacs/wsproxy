# Web-socket gateway

Service taking care of the management (and use) of stateful web-socket connections on behalf of clustered applications with with stateless back-ends.

## Endpoints provided by the gateway service

* `GET /connect`
  
  for client devices to open a web-socket connection
  
* `POST /message/${connectionId}`
  
  for application back-ends to send message over a web-socket connection

  (Client devices send messages to the back-ends using the web-socket connections between them and the gateway service.)

## Endpoints the gateway service expects the application to provide

* `GET /ws/connect`
    
  The service relays to this endpoint all requests coming in
  at its `GET /connect` end-point as they are. This endpoint
  is expected to authenticate the requests and return HTTP status `200` in the case of successful authentication (HTTP 401 in case of unsuccessful authentication).

* `POST /ws/disconnected`

  notifies of connections lost by the gateway on a best-effort basis

* `POST /ws/message`

  The gateway service relays to this end-point messages it receives from clients
