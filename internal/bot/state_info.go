package bot

import (
	"fmt"
	"log"
	"time"

	"github.com/Henry-Case-dev/luna_bot/internal/bot/prompts"
)

// sendStateInfo отправляет сообщение с текущим виртуальным состоянием бота
func (b *Bot) sendStateInfo(chatID int64) {
	// Получаем данные состояния через StateProvider
	if b.stateProvider == nil {
		b.sendTemporaryMessage(chatID, "📊 Система состояний не инициализирована.", 30*time.Second)
		return
	}

	stateData := b.stateProvider.CollectState(chatID, 0)

	// Получаем часовой пояс из конфига
	tz := b.config.TimeZone
	if tz == "" {
		tz = "Europe/Moscow"
	}

	// Локальное время
	loc, err := time.LoadLocation(tz)
	if err != nil {
		log.Printf("[StateInfo] Ошибка загрузки часового пояса %s: %v, использую Local", tz, err)
		loc = time.Local
	}
	localTime := time.Now().In(loc)

	// Формируем сообщение
	msg := "📊 Состояние Luna:\n"
	msg += fmt.Sprintf("🕐 Локальное время: %s (%s)\n", localTime.Format("15:04"), tz)

	presence := stateData.Presence
	mood := stateData.Mood

	// Сон
	if presence != nil && presence.Asleep {
		if presence.NightAwake {
			msg += "😴 Сон: да (ночное пробуждение)\n"
		} else {
			msg += "😴 Сон: да (спит)\n"
		}
	} else {
		msg += "😴 Сон: нет\n"
	}

	// Занятость
	if presence != nil && presence.IsBusy {
		msg += fmt.Sprintf("💼 Занятость: %s", presence.BusyLabel)
		if presence.BusyUntil != "" {
			msg += fmt.Sprintf(" (до %s)", presence.BusyUntil)
		}
		msg += "\n"
	} else {
		msg += "💼 Занятость: нет\n"
	}

	// Онлайн
	if presence != nil {
		onlineStatus := "да"
		if !presence.Online {
			onlineStatus = "нет"
			if presence.Asleep && !presence.NightAwake {
				onlineStatus += " (спит)"
			} else if presence.IsBusy {
				onlineStatus += fmt.Sprintf(" (занята: %s)", presence.BusyLabel)
			} else {
				onlineStatus += " (не в сети)"
			}
		}
		msg += fmt.Sprintf("📶 Онлайн: %s\n", onlineStatus)
	} else {
		msg += "📶 Онлайн: неизвестно\n"
	}

	// Вероятность ответа
	prob, probDesc := computeResponseProbability(presence)
	msg += fmt.Sprintf("🎯 Вероятность ответа: ~%d%% (%s)\n", prob, probDesc)

	// Настроение
	if mood != nil {
		energyLabel := tierLabel(mood.Energy)
		irritLabel := tierLabel(mood.Irritability)
		affectLabel := tierLabel(mood.Affection)

		msg += fmt.Sprintf("⚡ Энергия: %s (%.2f)\n", energyLabel, mood.Energy)
		msg += fmt.Sprintf("😤 Раздражительность: %s (%.2f)\n", irritLabel, mood.Irritability)
		msg += fmt.Sprintf("💕 Привязанность: %s (%.2f)\n", affectLabel, mood.Affection)
	}

	// Отправляем как временное сообщение
	b.sendTemporaryMessage(chatID, msg, 1*time.Minute)
}

// computeResponseProbability вычисляет примерную вероятность ответа на основе состояния присутствия
func computeResponseProbability(presence *prompts.PresenceData) (int, string) {
	if presence == nil {
		return 40, "стандартный режим"
	}

	if presence.Asleep && !presence.NightAwake {
		return 0, "спит — ответов нет"
	}

	if presence.NightAwake {
		return 10, "ночное пробуждение — короткие ответы"
	}

	if presence.IsBusy && !presence.Online {
		return 15, "занята — редкие проверки"
	}

	if presence.IsBusy && presence.Online {
		return 25, "занята, но заглядывает в Telegram"
	}

	if presence.Online {
		return 70, "онлайн — активный диалог"
	}

	// Оффлайн, но не спит и не занята
	return 30, "оффлайн — может прочитать позже"
}

// tierLabel возвращает текстовую метку уровня: высокая/средняя/низкая
func tierLabel(val float64) string {
	if val >= 0.67 {
		return "высокая"
	}
	if val >= 0.33 {
		return "средняя"
	}
	return "низкая"
}
