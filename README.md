# Websocket gateway

Service taking care of the management (and use) of stateful websocket connections on behalf of clustered applications with with stateless backends.

## Endpoints provided by the gateway

* `POST /subscribe`<br/>
  for client devices to open a websocket connection
* `POST /message/${connectionId}`<br/>
  for application backends to send message over a websocket connection

## Endpoints the service expects the application to provide

* `POST /ws/connecting`
  * matches each `/subscribe` request incoming at the gateway<br/>
  * should implement authentication (and optionally some authorization)
* `POST /ws/disconnected`
  * notifies of connections lost by the gateway on a best-effort basis
* `POST /ws/message-received`
  * notifies of messages received by the gateway
