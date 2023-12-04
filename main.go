package main

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bradfitz/gomemcache/memcache"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
)

var (
	mc *memcache.Client
)

func main() {
	mc = memcache.New("localhost:11211")
	fmt.Println(getAlephiumPrice(0))
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
	} else if !errors.Is(err, memcache.ErrCacheMiss) {
		//Если при обращении к memcached пришла ошибка, но заключается она не в том, что значения по ключу нет
		//завершаем функцию, выдаем ошибку
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

		//Извлекаем курс с проверкой на возможную ошибку, исключаем панику из-за длины среза
		if len(r.Chart.Result) == 1 {
			baseRublePrice = r.Chart.Result[0].Meta.RegularMarketPrice
		} else {
			return 0, errors.New("wrong response format")
		}

		//Сохраняем полученное значение курса в memcached, сначала преобразуем float64 в []byte для записи значения
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], math.Float64bits(baseRublePrice))

		err = mc.Set(&memcache.Item{
			Key:        key,
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

// getAlephiumPrice - курс Alphium к USD с добавленной дельтой
// <- memcache
func getAlephiumPrice(delta float64) (float64, error) {
	type GetAlphiumPrice struct {
		Alephium struct {
			Usd float64 `json:"usd"`
		} `json:"alephium"`
	}

	var alphData GetAlphiumPrice
	var alphUSD float64

	// Memcache
	mcKey := "func/getAlpheniumPrice"
	mcKey = generateSHA1Hash(mcKey)
	mcGet, err := mc.Get(mcKey)
	if err != nil || mcGet == nil {
		client := &http.Client{}
		req, err := http.NewRequest(http.MethodGet, "https://api.coingecko.com/api/v3/simple/price?ids=alephium&vs_currencies=USD", nil)
		if err != nil {
			return alphUSD, err
		}

		resp, err := client.Do(req)
		if err != nil {
			return alphUSD, err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return alphUSD, err
		}

		err = json.Unmarshal(respBody, &alphData)
		if err != nil {
			return alphUSD, err
		}

		alphUSD = alphData.Alephium.Usd

		// Запись в Memcache
		mcBody := []byte(floatToString(alphUSD))
		mc.Set(&memcache.Item{Key: mcKey, Value: mcBody, Expiration: 60})
	} else {
		// Получение данных из Memcache
		alphUSD = stringToFloat(string(mcGet.Value))
	}

	// Добавляем биржевую вилку
	alphUSD += delta

	return alphUSD, nil
}

func stringToFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func floatToString(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func generateSHA1Hash(key string) string {
	bData := []byte(key)
	bHex := sha1.Sum(bData)
	strHex := hex.EncodeToString(bHex[:])
	return strHex
}
