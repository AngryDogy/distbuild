# filecache

Пакет `filecache` занимается хранением кеша файлов и определяет протокол передачи файлов между частями системы.

`filecache.Cache` управляет файлами и занимается контролем одновременного доступа. Реализация этого типа вам уже дана.

## Передача файлов

Тип `filecache.Handler` реализует handler, позволяющий заливать и скачивать файлы из кеша.

- Вызов `GET /file?id=123` должен возвращать содержимое файла с `id=123`.
- Вызов `PUT /file?id=123` должен заливать содержимое файла с `id=123`.

