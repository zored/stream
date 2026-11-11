package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	weatherPollEvery = 20 * time.Minute
	weatherRetry     = time.Minute
	weatherStale     = 90 * time.Minute
	weatherBrowse    = 5 * time.Second
	weatherDays      = 3
)

type weatherHour struct {
	at        time.Time
	temp, app float64
	code      int
	day       bool
}

type weatherSlot struct {
	label string
	hour  weatherHour
}

var weatherParts = []struct {
	name string
	hour int
}{{"утро", 8}, {"день", 13}, {"вечер", 18}, {"ночь", 23}}

var weekdays = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

type weather struct {
	vars     *vars
	url      string
	http     *http.Client
	mu       sync.Mutex
	now      weatherHour
	hours    []weatherHour
	fetched  time.Time
	pick     int
	deadline time.Time
	frame    frame
}

func newWeather(v *vars, lat, lon float64, proxy string) (*weather, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("weather proxy: %w", err)
		}
		transport.Proxy = http.ProxyURL(u)
	}
	return &weather{vars: v, http: &http.Client{Timeout: 20 * time.Second, Transport: transport},
		url: fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%g&longitude=%g&timezone=auto&forecast_days=%d"+
			"&current=temperature_2m,apparent_temperature,weather_code,is_day"+
			"&hourly=temperature_2m,apparent_temperature,weather_code,is_day", lat, lon, weatherDays)}, nil
}

type openMeteo struct {
	UTCOffset int `json:"utc_offset_seconds"`
	Current   struct {
		Time  string  `json:"time"`
		Temp  float64 `json:"temperature_2m"`
		App   float64 `json:"apparent_temperature"`
		Code  int     `json:"weather_code"`
		IsDay int     `json:"is_day"`
	} `json:"current"`
	Hourly struct {
		Time  []string  `json:"time"`
		Temp  []float64 `json:"temperature_2m"`
		App   []float64 `json:"apparent_temperature"`
		Code  []int     `json:"weather_code"`
		IsDay []int     `json:"is_day"`
	} `json:"hourly"`
}

func parseOpenMeteo(data []byte) (weatherHour, []weatherHour, error) {
	var r openMeteo
	if err := json.Unmarshal(data, &r); err != nil {
		return weatherHour{}, nil, err
	}
	loc := time.FixedZone("", r.UTCOffset)
	at := func(s string) (time.Time, error) { return time.ParseInLocation("2006-01-02T15:04", s, loc) }
	t, err := at(r.Current.Time)
	if err != nil {
		return weatherHour{}, nil, err
	}
	now := weatherHour{at: t, temp: r.Current.Temp, app: r.Current.App, code: r.Current.Code, day: r.Current.IsDay == 1}
	h := r.Hourly
	if len(h.Temp) != len(h.Time) || len(h.App) != len(h.Time) || len(h.Code) != len(h.Time) || len(h.IsDay) != len(h.Time) {
		return weatherHour{}, nil, fmt.Errorf("open-meteo: hourly arrays differ in length")
	}
	hours := make([]weatherHour, len(h.Time))
	for i, s := range h.Time {
		if hours[i].at, err = at(s); err != nil {
			return weatherHour{}, nil, err
		}
		hours[i].temp, hours[i].app, hours[i].code, hours[i].day = h.Temp[i], h.App[i], h.Code[i], h.IsDay[i] == 1
	}
	return now, hours, nil
}

func weatherLabel(t time.Time, part string) string {
	return fmt.Sprintf("%s %d %s", weekdays[t.Weekday()], t.Day(), part)
}

func weatherPartOf(hour int) string {
	switch {
	case hour >= 5 && hour < 11:
		return "утро"
	case hour >= 11 && hour < 17:
		return "день"
	case hour >= 17 && hour < 23:
		return "вечер"
	}
	return "ночь"
}

// weatherSlots: now, then every part of the day after now while the hourly data lasts.
func weatherSlots(now weatherHour, hours []weatherHour) []weatherSlot {
	slots := []weatherSlot{{label: weatherLabel(now.at, weatherPartOf(now.at.Hour())), hour: now}}
	byTime := map[time.Time]weatherHour{}
	for _, h := range hours {
		byTime[h.at] = h
	}
	if len(hours) == 0 {
		return slots
	}
	y, m, d := now.at.Date()
	for day := 0; ; day++ {
		found := false
		for _, p := range weatherParts {
			t := time.Date(y, m, d+day, p.hour, 0, 0, 0, now.at.Location())
			if t.After(hours[len(hours)-1].at) {
				return slots
			}
			h, ok := byTime[t]
			if !ok {
				continue
			}
			found = true
			if t.After(now.at) {
				slots = append(slots, weatherSlot{label: weatherLabel(t, p.name), hour: h})
			}
		}
		if !found && day > 0 {
			return slots
		}
	}
}

func weatherIcon(code int, day bool) string {
	switch {
	case code == 0 || code == 1:
		if day {
			return "sun"
		}
		return "moon"
	case code == 2:
		if day {
			return "cloud-sun"
		}
		return "cloud-moon"
	case code == 3:
		return "clouds"
	case code == 45 || code == 48:
		return "cloud-fog2"
	case code >= 51 && code <= 55:
		return "cloud-drizzle"
	case code == 56 || code == 57 || code == 66 || code == 67:
		return "cloud-sleet"
	case code == 61 || code == 63 || code == 80 || code == 81:
		return "cloud-rain"
	case code == 65 || code == 82:
		return "cloud-rain-heavy"
	case code >= 71 && code <= 77, code == 85 || code == 86:
		return "cloud-snow"
	case code >= 95:
		return "cloud-lightning-rain"
	}
	return "cloud"
}

func weatherDegrees(t float64) string {
	return fmt.Sprint(int(math.Round(t)))
}

func weatherSVG(s weatherSlot, stale bool) string {
	color, feels := "#fff", gray
	if stale {
		color = gray
	}
	top := ""
	if s.label != "" {
		top = fmt.Sprintf(`<text x="50" y="12" text-anchor="middle" font-family="%s" font-size="12" fill="%s">%s</text>`,
			stripFont, color, html.EscapeString(s.label))
	}
	return dataURI(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` +
		iconAt(weatherIcon(s.hour.code, s.hour.day), color, 50, iconCY, iconSize) + top +
		fmt.Sprintf(`<text x="50" y="%g" text-anchor="middle" font-family="%s" font-size="%g" fill="%s">%s <tspan fill="%s">%s</tspan></text>`,
			captionY, stripFont, captionSize, color, weatherDegrees(s.hour.temp), feels, weatherDegrees(s.hour.app)) + `</svg>`)
}

func (w *weather) fetch() error {
	resp, err := w.http.Get(w.url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var buf []byte
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("open-meteo: %s", resp.Status)
	}
	if buf, err = io.ReadAll(resp.Body); err != nil {
		return err
	}
	now, hours, err := parseOpenMeteo(buf)
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.now, w.hours, w.fetched = now, hours, time.Now()
	w.mu.Unlock()
	w.publish()
	return nil
}

func (w *weather) run() {
	wait := weatherRetry
	for {
		if err := w.fetch(); err != nil {
			log.Printf("weather: %v", err)
			time.Sleep(wait)
			wait = min(wait*2, weatherPollEvery)
			continue
		}
		wait = weatherRetry
		time.Sleep(weatherPollEvery)
	}
}

func (w *weather) browsing() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.browsingLocked()
}

func (w *weather) browsingLocked() bool {
	return time.Now().Before(w.deadline)
}

func (w *weather) publish() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fetched.IsZero() {
		return
	}
	slots := weatherSlots(w.now, w.hours)
	pick := 0
	if w.browsingLocked() {
		pick = min(w.pick, len(slots)-1)
	}
	stale := time.Since(w.fetched) > weatherStale
	s := slots[pick]
	if !w.browsingLocked() {
		s.label = ""
	}
	key := fmt.Sprintf("%s|%v|%d|%d|%g|%g|%v", s.label, s.hour.at, s.hour.code, pick, s.hour.temp, s.hour.app, stale)
	w.vars.set("weather_svg", w.frame.render(key, func() string { return weatherSVG(s, stale) }))
}

func (w *weather) press() error {
	w.mu.Lock()
	if w.browsingLocked() {
		w.deadline = time.Time{}
	} else {
		w.pick, w.deadline = 0, time.Now().Add(weatherBrowse)
	}
	w.mu.Unlock()
	w.publish()
	return nil
}

// turn walks the forecast while the tile's browse window is open.
func (w *weather) turn(direction int) bool {
	w.mu.Lock()
	if !w.browsingLocked() {
		w.mu.Unlock()
		return false
	}
	n := len(weatherSlots(w.now, w.hours))
	w.pick = max(0, min(n-1, w.pick+direction))
	w.deadline = time.Now().Add(weatherBrowse)
	w.mu.Unlock()
	w.publish()
	return true
}
