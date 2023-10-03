package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/bradfitz/gomemcache/memcache"
	"log"
	"math"
	"net/http"
)

const (
	//Адрес memcached сервера
	memcachedServer = "localhost:11211"

	//Время хранения значения курса в секундах
	priceExpiration = 60

	//Адрес запроса биржи
	url = "https://query1.finance.yahoo.com/v8/finance/chart/RUB=X?includePrePost=false&interval=5m&useYfid=true&range=1d&corsDomain=finance.yahoo.com&.tsrc=finance"
)

var (
	mc *memcache.Client
)

func getRublePrice(delta float64) (float64, error) {
	var key = "roublePrice"
	//Попытка получить курс из memcached
	roublePrice, err := getFloat64FromMemcachedByKey(key)
	if err != nil {
		//Если ошибка заключается в том, что memcached не содержит такого значения
		if errors.Is(err, memcache.ErrCacheMiss) {
			//Получение курса от биржи и сохранение его в memcached
			roublePrice, err = getRublePriceFromServerAndSaveToMemcached(key)
		} else {
			return 0, err
		}
	}

	//Увеличение курса на величину delta
	return delta + roublePrice, err
}

// Получение значения курса рубля из memcached по заданному ключу
func getFloat64FromMemcachedByKey(key string) (float64, error) {
	//если memcached клиент еще не проинициализирован, то инициализируем его
	if mc == nil {
		err := initMemcached()
		if err != nil {
			return 0, err
		}
	}

	//Если такого значения нет, выбрасываем ошибку
	item, err := mc.Get(key)
	if err != nil {
		return 0, err
	}

	log.Println("rouble price got from memcached")
	return bytesToFloat64(item.Value), nil
}

// Получение значения курса рубля от биржи и сохранение его в memcached
func getRublePriceFromServerAndSaveToMemcached(key string) (float64, error) {
	roublePrice, err := getRublePriceFromServer()
	if err != nil {
		return 0, err
	}

	err = addFloat64ToMemcachedWithKey(key, roublePrice)

	return roublePrice, err
}

// Получение курса рубля от биржи, работа с JSON, структурой ответа
func getRublePriceFromServer() (float64, error) {
	resp, err := http.Get(url)
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

	//Извлекаем цену с проверкой на возможную ошибку, исключаем получение паники из-за длины среза
	var roublePrice float64
	if len(r.Chart.Result) == 1 {
		roublePrice = r.Chart.Result[0].Meta.RegularMarketPrice
	} else {
		err = errors.New("wrong response format")
	}

	//Если в JSON пришла ошибка, передаем ее выше
	if len(r.Chart.Error) != 0 {
		err = errors.New(r.Chart.Error)
	}

	log.Println("rouble price got from server")
	return roublePrice, err
}

// Сохранение float64 в memcached с заданным ключом
func addFloat64ToMemcachedWithKey(key string, price float64) error {
	//если memcached клиент еще не проинициализирован, то инициализируем его
	if mc == nil {
		err := initMemcached()
		if err != nil {
			return err
		}
	}

	return mc.Set(&memcache.Item{
		Key: key,
		//Для записи в memcached конвертируем float64 в []byte
		Value:      float64ToBytes(price),
		Expiration: priceExpiration,
	})
}

// Создание memcached клиент, проверка его для уверенности в том, что подключение в норме
func initMemcached() error {
	mc = memcache.New(memcachedServer)
	return mc.Ping()
}

// Конвертация float64 в байты для записи курса в memcached
func float64ToBytes(f float64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], math.Float64bits(f))
	return buf[:]
}

// Конвертация байтов обратно во float64 для вывода значения из memcached
func bytesToFloat64(buf []byte) float64 {
	return math.Float64frombits(binary.BigEndian.Uint64(buf))
}
