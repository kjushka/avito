# Тестовое задание в Avito Merchant Experience

### Сборка и запуск
1. docker build . -f ./backend-d -t backend-d
2. docker-compose up

### Пояснения к проекту

* Было принято решение не обрабатывать каждую строку таблицы в отдельном потоке, так как создание горутины заняло бы больше времени, чем обработать 100 таких же строк. Так же это позволило оптимизировать процесс выполнения запросов к бд - на каждые 100 строк - один запрос на сохранение/изменение и один на удаление.

* В связи с оптимизацией запросов я не реализовал разделение итоговых данных на добавленные и измененные, так как эта процедура фактически реализовывалась через один запрос. Было два варианта решения этой проблемы: получить изначально все айди товаров для данного продавца и любым способом поиска (допустим бинарным) определять, существует уже товар с таким идентификатором или нет, и выполнять для каждого товара отдельный запрос сначала на проверку, затем на добавление и изменение. Оба способа занимают гораздо больше времение на исполнение, в связи с чем было решено оставить текущую реализацию.

