# api

Пакет api реализует протокол, по которому общаются компоненты системы.

Этот пакет не занимается передачей файлов и артефактов, соответствующие функции находятся в
пакетах `filecache` и `artifact`.

## Worker <-> Coordinator

- Worker и Coordinator общаются через один запрос `POST /heartbeat`.
- Worker посылает `HeartbeatRequest` и получает в ответ `HeartbeatResponse`.
- Запрос и ответ передаются в формате json.
- Ошибка обработки heartbeat передаётся как текстовая строка.

## Client <-> Coordinator

Client и Coordinator общаются через два вызова.

- `POST /build` - стартует новый билд. 
  * Client посылает в Body запроса json c описанием сборки. 
  * Coordinator стримит в body ответа json сообщения, описывающие прогресс сборки.
  * _Тут можно было бы использовать websocket, но нас устраивает более простое решение._
  * Чтобы послать клиенту неполный body, нужно использовать метод `Flush`. 
    Прочитайте про [`http.ResponseController`](https://pkg.go.dev/net/http#ResponseController) и используйте его.
  * Первым сообщением в ответе Coordinator присылает `buildID`.

- `POST /signal?build_id=12345` - посылает сигнал бегущему билду.
  * Запрос и ответ передаются в формате json.
