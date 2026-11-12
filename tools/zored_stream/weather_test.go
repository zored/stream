package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestWeatherSlots(t *testing.T) {
	var times, temps, codes, days []string
	start := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 48; i++ {
		times = append(times, `"`+start.Add(time.Duration(i)*time.Hour).Format("2006-01-02T15:04")+`"`)
		temps = append(temps, fmt.Sprint(i))
		codes = append(codes, "0")
		days = append(days, "1")
	}
	j := func(a []string) string { return "[" + strings.Join(a, ",") + "]" }
	data := fmt.Sprintf(`{"utc_offset_seconds":10800,"current":{"time":"2026-11-05T15:45","temperature_2m":3.6,"apparent_temperature":-0.4,"weather_code":2,"is_day":1},
		"hourly":{"time":%s,"temperature_2m":%s,"apparent_temperature":%s,"weather_code":%s,"is_day":%s}}`, j(times), j(temps), j(temps), j(codes), j(days))
	now, hours, err := parseOpenMeteo([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range weatherSlots(now, hours) {
		got = append(got, fmt.Sprintf("%s=%s", s.label, weatherDegrees(s.hour.temp)))
	}
	want := "чт 5=4,чт 5 вечер=18,чт 5 ночь=23,пт 6 утро=32,пт 6 день=37,пт 6 вечер=42,пт 6 ночь=47"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %s", strings.Join(got, ","))
	}
}

func TestWeatherIcon(t *testing.T) {
	for _, c := range []struct {
		code int
		day  bool
		want string
	}{{0, true, "sun"}, {1, false, "moon"}, {2, true, "cloud-sun"}, {3, true, "clouds"}, {45, true, "cloud-fog2"},
		{53, true, "cloud-drizzle"}, {61, true, "cloud-rain"}, {65, true, "cloud-rain-heavy"}, {66, true, "cloud-sleet"},
		{73, true, "cloud-snow"}, {86, true, "cloud-snow"}, {95, true, "cloud-lightning-rain"}} {
		if got := weatherIcon(c.code, c.day); got != c.want {
			t.Errorf("%d %v: got %s, want %s", c.code, c.day, got, c.want)
		}
		iconBody(weatherIcon(c.code, c.day))
	}
	if weatherDegrees(-0.4) != "0" || weatherDegrees(-2.6) != "-3" {
		t.Fatal("degrees")
	}
}
