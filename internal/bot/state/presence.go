package state

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

// PresencePattern represents the type of presence behaviour.
type PresencePattern string

const (
	PatternPhoneAttached      PresencePattern = "phone-attached"
	PatternBurstChecker       PresencePattern = "burst-checker"
	PatternRareChecker        PresencePattern = "rare-checker"
	PatternEveningOnly        PresencePattern = "evening-only"
	PatternPhoneAttachedNight PresencePattern = "phone-attached-night"
)

// PresenceProfile holds deterministic presence configuration.
type PresenceProfile struct {
	Pattern            PresencePattern
	SleepFrom          int     // 0..23, hour she goes to bed (local tz)
	SleepTo            int     // 0..23, hour she wakes up (local tz)
	CheckEveryMin      int     // average interval between Telegram checks (minutes)
	OnlineWindowMin    int     // minutes she stays "online" per visit
	OfflineReplyChance float64 // 0..1, base probability of replying while offline (push notification)
	NightWakeChance    float64 // 0..1, probability of waking up at night for incoming messages
}

// PresenceState represents the current presence state.
type PresenceState struct {
	Online           bool
	Asleep           bool
	NightAwake       bool
	NextCheckSec     int
	LocalHour        int
	Hint             string
	Busy             *BusyInfo
	NotificationSeen bool
}

// BusyInfo describes an active busy slot.
type BusyInfo struct {
	Label         string
	Until         string
	CheckAfterMin int
}

// BusySlot defines a recurring time slot when the persona is busy (work, university, gym, etc.).
type BusySlot struct {
	Label         string
	From          string      // "HH:MM" local time
	To            string      // "HH:MM" local time
	Days          []time.Weekday // empty means every day
	CheckAfterMin [2]int      // min, max minutes after the slot ends before checking Telegram
}

// seqRng is a deterministic pseudo-random generator matching the TypeScript reference
// formula: ((seed*9301 + n*49297) % 233280) / 233280
func seqRand(seed int64, n int) float64 {
	return float64((seed*9301+int64(n)*49297)%233280) / 233280.0
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ComputePresenceProfile generates a deterministic PresenceProfile from a seed.
// The seed should be derived from persona name + age (or any stable identifier).
func ComputePresenceProfile(seed int64, sleepFrom, sleepTo int, nightWakeChance float64) *PresenceProfile {
	r := func(n int) float64 { return seqRand(seed, n) }

	patterns := []PresencePattern{
		PatternPhoneAttached,
		PatternBurstChecker,
		PatternRareChecker,
		PatternEveningOnly,
		PatternPhoneAttachedNight,
	}
	pattern := patterns[int(r(1)*float64(len(patterns)))]

	var checkEveryMin int
	switch pattern {
	case PatternPhoneAttached:
		checkEveryMin = 3 + int(r(4)*5)
	case PatternBurstChecker:
		checkEveryMin = 15 + int(r(4)*20)
	case PatternRareChecker:
		checkEveryMin = 60 + int(r(4)*60)
	case PatternEveningOnly:
		checkEveryMin = 45 + int(r(4)*30)
	case PatternPhoneAttachedNight:
		checkEveryMin = 10 + int(r(4)*15)
	}

	var onlineWindowMin int
	switch pattern {
	case PatternPhoneAttached:
		onlineWindowMin = 30 + int(r(5)*60)
	case PatternBurstChecker:
		onlineWindowMin = 2 + int(r(5)*4)
	case PatternRareChecker:
		onlineWindowMin = 5 + int(r(5)*10)
	case PatternEveningOnly:
		onlineWindowMin = 60 + int(r(5)*90)
	case PatternPhoneAttachedNight:
		onlineWindowMin = 20 + int(r(5)*40)
	}

	var offlineReplyChance float64
	switch pattern {
	case PatternPhoneAttached:
		offlineReplyChance = 0.85
	case PatternBurstChecker:
		offlineReplyChance = 0.5
	case PatternRareChecker:
		offlineReplyChance = 0.25
	case PatternEveningOnly:
		offlineReplyChance = 0.3
	case PatternPhoneAttachedNight:
		offlineReplyChance = 0.55
	}

	return &PresenceProfile{
		Pattern:            pattern,
		SleepFrom:          sleepFrom,
		SleepTo:            sleepTo,
		CheckEveryMin:      checkEveryMin,
		OnlineWindowMin:    onlineWindowMin,
		OfflineReplyChance: offlineReplyChance,
		NightWakeChance:    nightWakeChance,
	}
}

// IsHourInRange returns true if hour h falls within [from, to) in circular 24h space.
// Supports ranges that cross midnight (e.g. from=23, to=8 covers 23,0,1,2,3,4,5,6,7).
func IsHourInRange(h, from, to int) bool {
	if from == to {
		return false
	}
	if from < to {
		return h >= from && h < to
	}
	return h >= from || h < to
}

// parseTime parses "HH:MM" into minutes since midnight. Returns 0, false on error.
func parseTime(t string) (int, bool) {
	var h, m int
	n, err := fmt.Sscanf(t, "%d:%d", &h, &m)
	if err != nil || n != 2 {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// weekdayIndex converts time.Weekday to zero-based index where Monday=0 (matching TS scheme).
func weekdayIndex(wd time.Weekday) int {
	switch wd {
	case time.Monday:
		return 0
	case time.Tuesday:
		return 1
	case time.Wednesday:
		return 2
	case time.Thursday:
		return 3
	case time.Friday:
		return 4
	case time.Saturday:
		return 5
	case time.Sunday:
		return 6
	default:
		return 0
	}
}

// previousWeekday returns the day before the given weekday.
func previousWeekday(wd time.Weekday) time.Weekday {
	if wd == time.Sunday {
		return time.Saturday
	}
	return wd - 1
}

// dayAllowed checks if a busy slot applies on the given weekday.
func dayAllowed(slot BusySlot, day time.Weekday) bool {
	if len(slot.Days) == 0 {
		return true
	}
	for _, d := range slot.Days {
		if d == day {
			return true
		}
	}
	return false
}

// ActiveBusySlot returns the first busy slot that is active at the given minute of day.
// Returns nil if no slot is active.
func ActiveBusySlot(slots []BusySlot, weekday time.Weekday, minuteOfDay int) *BusySlot {
	for i := range slots {
		slot := &slots[i]
		from, ok := parseTime(slot.From)
		if !ok {
			continue
		}
		to, ok := parseTime(slot.To)
		if !ok || from == to {
			continue
		}

		if from < to {
			if dayAllowed(*slot, weekday) && minuteOfDay >= from && minuteOfDay < to {
				return slot
			}
			continue
		}

		// Slot crosses midnight (from > to)
		if dayAllowed(*slot, weekday) && minuteOfDay >= from {
			return slot
		}
		if dayAllowed(*slot, previousWeekday(weekday)) && minuteOfDay < to {
			return slot
		}
	}
	return nil
}

// busySlotRemaining returns remaining minutes until the slot ends and its end time string.
func busySlotRemaining(slot BusySlot, minuteOfDay int) (int, string) {
	from, _ := parseTime(slot.From)
	to, _ := parseTime(slot.To)
	if from < to {
		return to - minuteOfDay, slot.To
	}
	// Crosses midnight
	if minuteOfDay >= from {
		return 1440 - minuteOfDay + to, slot.To
	}
	return to - minuteOfDay, slot.To
}

// randomCheckAfter returns a random check-after value from the slot's range.
func randomCheckAfter(slot BusySlot) int {
	lo, hi := slot.CheckAfterMin[0], slot.CheckAfterMin[1]
	if lo == 0 && hi == 0 {
		lo, hi = 5, 15
	}
	minVal := clampInt(lo, 1, hi)
	maxVal := clampInt(hi, max(lo, 1), 999)
	if maxVal < minVal {
		minVal, maxVal = maxVal, minVal
	}
	if maxVal == minVal {
		return minVal
	}
	return minVal + rand.Intn(maxVal-minVal+1)
}

// minutesUntil calculates minutes from (hour:minute) until (targetHour:targetMinute).
// Handles wrapping across midnight.
func minutesUntil(hour, minute, targetHour, targetMinute int) int {
	now := hour*60 + minute
	target := targetHour*60 + targetMinute
	if target > now {
		return target - now
	}
	return target + 1440 - now
}

// localParts returns the current local hour, minute, and weekday in the given IANA timezone.
func localParts(tz string) (hour, minute int, weekday time.Weekday) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		now := time.Now()
		return now.Hour(), now.Minute(), now.Weekday()
	}
	now := time.Now().In(loc)
	return now.Hour(), now.Minute(), now.Weekday()
}

// ComputePresenceState computes the current presence state deterministically
// based on profile, timezone, busy slots, message context, and flags.
//
// Parameters:
//   - profile: presence configuration from ComputePresenceProfile
//   - tz: IANA timezone string (e.g. "Europe/Moscow")
//   - busySlots: recurring busy schedule
//   - lastUserMsgTs: timestamp of the last message from the human
//   - lastHerReplyTs: timestamp of her last reply
//   - recentExchangeCount: number of back-and-forth exchanges in the last 30 minutes
//   - forcedWake: when true, forces her to be online (e.g. direct ping)
//   - conflictCold: when true, she is in a conflict cold period (ignoring)
func ComputePresenceState(
	profile *PresenceProfile,
	tz string,
	busySlots []BusySlot,
	lastUserMsgTs time.Time,
	lastHerReplyTs time.Time,
	recentExchangeCount int,
	forcedWake bool,
	conflictCold bool,
) *PresenceState {
	localHour, localMinute, weekday := localParts(tz)
	minuteOfDay := localHour*60 + localMinute

	asleep := IsHourInRange(localHour, profile.SleepFrom, profile.SleepTo)

	var busy *BusyInfo
	var activeSlot *BusySlot
	if !asleep {
		activeSlot = ActiveBusySlot(busySlots, weekday, minuteOfDay)
		if activeSlot != nil {
			_, until := busySlotRemaining(*activeSlot, minuteOfDay)
			checkAfter := randomCheckAfter(*activeSlot)
			busy = &BusyInfo{
				Label:         activeSlot.Label,
				Until:         until,
				CheckAfterMin: checkAfter,
			}
		}
	}

	nightAwake := forcedWake || (asleep && profile.NightWakeChance > 0 && rand.Float64() < profile.NightWakeChance)

	now := time.Now()
	inActiveDialog := recentExchangeCount >= 3 &&
		!lastHerReplyTs.IsZero() &&
		now.Sub(lastHerReplyTs) < 15*time.Minute &&
		now.Sub(lastUserMsgTs) < 5*time.Minute

	var online bool
	var nextCheckSec int
	var notificationSeen bool

	switch {
	case conflictCold && !forcedWake:
		online = false
		nextCheckSec = 3600

	case asleep && !nightAwake:
		online = false
		hoursToWake := float64(profile.SleepTo - localHour)
		if hoursToWake < 0 {
			hoursToWake += 24
		}
		if hoursToWake == 0 {
			hoursToWake = 0.5
		}
		nextCheckSec = int(hoursToWake*3600) + rand.Intn(1800)

	case activeSlot != nil && !forcedWake:
		busyMul := 1.0
		activeDialogMul := 1.0
		if inActiveDialog {
			activeDialogMul = 0.35
		}

		rawMinCheck := activeSlot.CheckAfterMin[0]
		rawMaxCheck := activeSlot.CheckAfterMin[1]
		if rawMinCheck == 0 && rawMaxCheck == 0 {
			rawMinCheck = 5
			rawMaxCheck = 15
		}

		minCheck := max(1, int(math.Round(float64(rawMinCheck)*busyMul*activeDialogMul)))
		maxCheck := max(minCheck, int(math.Round(float64(rawMaxCheck)*busyMul*activeDialogMul)))

		remainingMin, _ := busySlotRemaining(*activeSlot, minuteOfDay)

		if maxCheck <= 5 {
			cycleMin := max(1, int(math.Round(float64(minCheck+maxCheck)/2)))
			minuteOfCycle := minuteOfDay % max(1, cycleMin)
			onlineMin := max(1, min(2, cycleMin/2))
			online = minuteOfCycle < onlineMin
			if !online {
				nextCheckSec = (cycleMin - minuteOfCycle) * 60
			}
			if busy != nil {
				busy.CheckAfterMin = cycleMin
			}
		} else {
			online = false
			checkAfterMin := minCheck
			if maxCheck > minCheck {
				checkAfterMin = minCheck + rand.Intn(maxCheck-minCheck+1)
			}
			activeDialogCapMin := 20
			waitMin := remainingMin + checkAfterMin
			if inActiveDialog {
				waitMin = min(waitMin, activeDialogCapMin)
			}
			nextCheckSec = waitMin * 60
			if busy != nil {
				busy.CheckAfterMin = checkAfterMin
			}
		}

	case forcedWake:
		online = true

	case inActiveDialog:
		online = true

	default:
		isEvening := localHour >= 18 || localHour < profile.SleepFrom
		isNightOwl := localHour >= 22 || localHour < 8

		switch {
		case profile.Pattern == PatternEveningOnly && !isEvening:
			online = false
			minutesToEvening := max(1, minutesUntil(localHour, localMinute, 18, 0))
			capMin := 45
			nextCheckSec = min(minutesToEvening, capMin)*60 + rand.Intn(600)

		case profile.Pattern == PatternPhoneAttachedNight && !isNightOwl:
			online = false
			minutesToNight := max(1, minutesUntil(localHour, localMinute, 22, 0))
			capMin := 45
			nextCheckSec = min(minutesToNight, capMin)*60 + rand.Intn(600)

		default:
			onlineProb := float64(profile.OnlineWindowMin) / float64(profile.OnlineWindowMin+profile.CheckEveryMin)
			online = rand.Float64() < onlineProb
			if !online {
				notificationSeen = rand.Float64() < profile.OfflineReplyChance
				if notificationSeen {
					nextCheckSec = 60 + rand.Intn(4*60)
				} else {
					nextCheckSec = max(30, rand.Intn(profile.CheckEveryMin*60))
				}
			}
		}
	}

	hint := buildHint(buildHintInput{
		forcedWake:        forcedWake,
		conflictCold:      conflictCold,
		asleep:            asleep,
		nightAwake:        nightAwake,
		busy:              busy,
		online:            online,
		inActiveDialog:    inActiveDialog,
		notificationSeen:  notificationSeen,
		localHour:         localHour,
		nextCheckSec:      nextCheckSec,
		pattern:           profile.Pattern,
	})

	return &PresenceState{
		Online:           online,
		Asleep:           asleep,
		NightAwake:       nightAwake,
		NextCheckSec:     nextCheckSec,
		LocalHour:        localHour,
		Hint:             hint,
		Busy:             busy,
		NotificationSeen: notificationSeen,
	}
}

type buildHintInput struct {
	forcedWake       bool
	conflictCold     bool
	asleep           bool
	nightAwake       bool
	busy             *BusyInfo
	online           bool
	inActiveDialog   bool
	notificationSeen bool
	localHour        int
	nextCheckSec     int
	pattern          PresencePattern
}

func buildHint(in buildHintInput) string {
	switch {
	case in.forcedWake:
		return "FORCED WAKE — ты получила уведомление/разбудили. Отвечай, даже если обычно спишь или занята. Не упоминай что тебя \"разбудили\", просто отвечай как обычно."

	case in.conflictCold:
		return "КОНФЛИКТ. Ты в режиме молчания. Отвечай очень редко, сухо: \".\", \"и?\", \"ну?\", \"что\". Никаких смайликов. Можно вообще игнорить."

	case in.asleep && !in.nightAwake:
		return fmt.Sprintf("СПИШЬ (по местному %d:00). Если отвечаешь — то заспанно, односложно, либо вообще игнорь до утра.", in.localHour)

	case in.nightAwake:
		return fmt.Sprintf("НОЧНОЕ ПРОБУЖДЕНИЕ (по местному %d:00). Ты проснулась посреди ночи — возможно от уведомления, возможно сама. ГОЛОВА НЕ ВАРИТ. Пишешь очень коротко, с опечатками, без заглавных, дезориентирована. Можешь снова заснуть посреди фразы — тогда обрываешь. Не ведёшь длинные разговоры. Если не хочешь — просто \"спи\" или \"потом\".", in.localHour)

	case in.busy != nil:
		if in.busy.CheckAfterMin <= 5 {
			return fmt.Sprintf("На %s (до %s), но скучно — периодически заходишь в Telegram между делом. Можешь быстро ответить, но можешь и пропустить момент.", in.busy.Label, in.busy.Until)
		}
		return fmt.Sprintf("Сейчас занята: %s до %s. Не можешь нормально отвечать. После этого зайдёшь проверить Telegram через %d мин; сообщение будет прочитано и ответ будет только тогда.", in.busy.Label, in.busy.Until, in.busy.CheckAfterMin)

	case in.online:
		if in.inActiveDialog {
			return "АКТИВНЫЙ ДИАЛОГ. Отвечаешь относительно быстро. Если резко \"ушла\" — ОБЯЗАТЕЛЬНО оправдание (мама зашла / в туалет / еду / котик прыгнул на колени). Внезапно \"вышла из сети\" без причин — это палево, не делай так."
		}
		return "Сейчас в сети, но не залипала — просто заглянула в тг. Можешь зайти в диалог сразу или через 1-2 мин."

	default:
		nextCheckMin := int(math.Round(float64(in.nextCheckSec) / 60))
		if in.notificationSeen {
			return fmt.Sprintf("Сейчас НЕ в сети, но уведомление увидела. Зайдёшь в Telegram через ~%d мин и тогда ответишь, если захочешь.", nextCheckMin)
		}
		return fmt.Sprintf("Сейчас НЕ в сети (паттерн: %s). Уведомление не заметила; прочитаешь это через ~%d мин когда сама заглянешь в тг. Это нормально, реальные люди не сидят в тг 24/7.", in.pattern, nextCheckMin)
	}
}
