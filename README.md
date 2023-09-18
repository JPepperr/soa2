# Мафия

### Сервер
Запуск сервера:
```
docker-compose build
docker-compose up
```
После этого контейнер сервера запущен и слушает на порту 5050. Также поднят REST сервис на порту 6669. Там специально сделал так, чтобы при запросе нескольких юзеров аватарка не приезжала, иначе сложно глазами парсить ответ. При запросе конкретного юзера приезжает чесный ответ с аватаркой.

Примеры команд:
```
curl -X POST http://localhost:6669/user/jpepper -F "nickname=jpepper" -F "sex=male" -F "picture=@./ob.jpg" -F "email=jpepper@jpepper.ru" -H "Content-Type: multipart/form-data"
```
где `jpepper` пользователь, а `./ob.jpg` относительный путь к картинке.
```
curl -X GET http://localhost:6669/user/jpepper --output "test_bin_output"
```
данные пользователя с картинкой
```
curl -X GET "http://localhost:6669/users?ids=jpepper"
```
данные пользователя без картинки

---

Также запущен qraphql клиент на порту 7776. Удобнее всего запросы делать через браузерную страничку `http://localhost:7776/`

### Клиент
Для запуска клиента нужно выполнить из директории `client`:
```
go run .
```
