/*
[2026-07-08] :: 🚀 :: Added PingDNS, SingBoxAlive, SingBoxRoute rendering lines
[2026-07-02] :: 🚀 :: Initial status render with Russian emoji formatting
*/

package controller

import (
	"strconv"
	"strings"
)

// RenderStatus formats a Status snapshot into a human-readable Russian-emoji string.
func RenderStatus(s Status) string {
	var b strings.Builder

	b.WriteString("🤖 Статус\n")
	b.WriteString("━━━━━━━━━━━━━━━━\n")

	// Процесс
	if s.HasProcess {
		b.WriteString("🟢 Процесс: работает\n")
	} else {
		b.WriteString("🔴 Процесс: остановлен\n")
	}

	// Провайдер
	if s.Provider != nil {
		b.WriteString("📦 Провайдер: ")
		b.WriteString(s.Provider.Kind)
		b.WriteString(" · ")
		b.WriteString(s.Provider.RoomID)
		b.WriteString("\n")
	} else {
		b.WriteString("📦 Провайдер: не задан\n")
	}

	// Запущен
	b.WriteString("🕒 Запущен: ")
	if s.ProcessStarted != nil {
		b.WriteString(s.ProcessStarted.Local().Format("2006-01-02 15:04:05"))
	} else {
		b.WriteString("—")
	}
	b.WriteString("\n")

	// Остановлен
	b.WriteString("🛑 Остановлен: ")
	if s.ProcessStopped != nil {
		b.WriteString(s.ProcessStopped.Local().Format("2006-01-02 15:04:05"))
	} else {
		b.WriteString("—")
	}
	b.WriteString("\n")

	// Код выхода
	b.WriteString("🔢 Код выхода: ")
	if s.LastExitCode != nil {
		b.WriteString(strconv.Itoa(*s.LastExitCode))
	} else {
		b.WriteString("—")
	}
	b.WriteString("\n")

	// Перезапуск
	b.WriteString("🔁 Перезапуск: ")
	if s.Restarting {
		b.WriteString("да")
	} else {
		b.WriteString("нет")
	}
	b.WriteString("\n")

	// Ошибка
	b.WriteString("⚠️ Ошибка: ")
	if s.LastError != "" {
		b.WriteString(s.LastError)
	} else {
		b.WriteString("нет")
	}
	b.WriteString("\n")

	// Пинг DNS (always shown)
	b.WriteString("🌐 Пинг DNS (9.9.9.9): ")
	if s.PingDNS != "" {
		b.WriteString(s.PingDNS)
	} else {
		b.WriteString("—")
	}
	b.WriteString("\n")

	// Sing-Box status (client only: SingBoxAlive != nil)
	if s.SingBoxAlive != nil {
		b.WriteString("📦 Sing-Box: ")
		if *s.SingBoxAlive {
			b.WriteString("🟢 работает")
		} else {
			b.WriteString("🔴 не отвечает")
		}
		b.WriteString("\n")

		b.WriteString("🔀 Маршрут: ")
		if s.SingBoxRoute != "" {
			b.WriteString(s.SingBoxRoute)
		} else {
			b.WriteString("—")
		}
		b.WriteString("\n")
	}

	return b.String()
}
