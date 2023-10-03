package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bradfitz/gomemcache/memcache"
	"log"
	"math"
	"net/http"
)

func main() {
	fmt.Println(getRublePrice(-70))
}

func getRublePrice(delta float64) (float64, error) {
	const (
		//Адрес memcached сервера
		memcachedServer = "localhost:11211"

		//Время хранения значения курса в секундах
		priceExpiration = 60

		//Адрес запроса биржи
		url = "https://query1.finance.yahoo.com/v8/finance/chart/RUB=X?includePrePost=false&interval=5m&useYfid=true&range=1d&corsDomain=finance.yahoo.com&.tsrc=finance"

		key = "roublePrice"
	)

	var baseRublePrice float64

	//Попытка получить курс из memcached
	mc := memcache.New(memcachedServer)
	item, err := mc.Get(key)
	if err == nil {
		//Если нашлась запись в memcached, преобразуем его значение из среза байтов во float64, добавляем значение delta и выводим
		baseRublePrice = math.Float64frombits(binary.BigEndian.Uint64(item.Value))
		log.Println("значение курса рубля получено из memcached")
		return baseRublePrice + delta, err
	} else
	//Если при обращении к memcached пришла ошибка, но заключается она не в том, что значения по ключу нет
	if !errors.Is(err, memcache.ErrCacheMiss) {
		//Завершаем функцию, выдаем ошибку
		return 0, err
	} else {
		//Если ошибка получения значения из memcached заключается в том, что такого значения нет, получаем значение от биржи
		var resp *http.Response
		resp, err = http.Get(url)
		if err != nil {
			return 0, err
		}
		//Хотел уточнить при случае, нормальная ли практика в отложенном вызове не обрабатывать подобные ошибки? Или стоит их хотя бы логировать?
		defer resp.Body.Close()

		//Описываю интересующие меня поля в JSON ответа, чтобы в дальнейшем обработать их. Есть ли более адекватный способ достучаться до глубоко вложенных значений, чем этот?
		type response struct {
			Chart struct {
				Result []struct {
					Meta struct {
						RegularMarketPrice float64 `json:"regularMarketPrice"`
					} `json:"meta"`
				} `json:"result"`
				Error string `json:"error"`
			} `json:"chart"`
		}

		//Парсим JSON в объект описанной структуры
		var r response
		err = json.NewDecoder(resp.Body).Decode(&r)
		if err != nil {
			return 0, err
		}

		//Если в JSON пришла ошибка, выдаем ее
		if len(r.Chart.Error) != 0 {
			return 0, errors.New(r.Chart.Error)
		}

		//Извлекаем цену с проверкой на возможную ошибку, исключаем получение паники из-за длины среза
		if len(r.Chart.Result) == 1 {
			baseRublePrice = r.Chart.Result[0].Meta.RegularMarketPrice
		} else {
			return 0, errors.New("wrong response format")
		}

		//Сохраняем полученное значение курса в memcached, сначала преобразуем float64 в []byte для записи значения
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], math.Float64bits(baseRublePrice))

		err = mc.Set(&memcache.Item{
			Key: key,
			//Для записи в memcached конвертируем float64 в []byte
			Value:      buf[:],
			Expiration: priceExpiration,
		})
		if err != nil {
			return 0, err
		}

		//Если все в порядке, значение от сервера получено, записано в memcached, то добавляем дельту и выдаем его
		log.Println("значение курса рубля получено от биржи")
		return baseRublePrice + delta, err
	}
}
