package bot

import (
	"strings"
	"testing"
)

// newTestContextIsolator создаёт изолятор для тестов
func newTestContextIsolator() *ContextIsolator {
	return &ContextIsolator{bot: nil}
}

func TestStripSensitiveData_RemovesBioForNonTargetUser(t *testing.T) {
	ci := newTestContextIsolator()

	rawContext := `[MSG_ID:1]
[U100:Alice]
username: @alice
alias: Alice
bio: Alice is a developer from Moscow
date: 15:04 02.01 (Mon)
message: Привет всем!
[/MSG_ID:1]

[MSG_ID:2]
[U200:Bob]
username: @bob
alias: Bob
bio: Bob loves cats
date: 15:05 02.01 (Mon)
message: Всем привет!
[/MSG_ID:2]
`

	// keepUserID = 100 — сохраняем BIO только для Alice, удаляем Bob
	result := ci.StripSensitiveData(rawContext, 100)

	if strings.Contains(result, "bio: Bob") {
		t.Errorf("BIO для Bob (нецелевой пользователь) должно быть удалено")
	}
	if !strings.Contains(result, "bio: Alice") {
		t.Errorf("BIO для Alice (целевой пользователь) должно сохраниться")
	}
}

func TestStripSensitiveData_PreservesBioForTargetUser(t *testing.T) {
	ci := newTestContextIsolator()

	rawContext := `[MSG_ID:1]
[U300:Charlie]
username: @charlie
alias: Charlie
bio: Charlie is a musician
date: 15:04 02.01 (Mon)
message: Hey there
[/MSG_ID:1]
`

	// keepUserID = 300 — сохраняем BIO для Charlie
	result := ci.StripSensitiveData(rawContext, 300)

	if !strings.Contains(result, "bio: Charlie") {
		t.Errorf("BIO для целевого пользователя (300) должно сохраниться, получено: %s", result)
	}
}

func TestStripSensitiveData_RemovesAllBioInGeneralMode(t *testing.T) {
	ci := newTestContextIsolator()

	rawContext := `[MSG_ID:1]
[U400:Dave]
username: @dave
alias: Dave
bio: Dave is a writer
date: 15:04 02.01 (Mon)
message: Hello
[/MSG_ID:1]

[MSG_ID:2]
[U500:Eve]
username: @eve
alias: Eve
bio: Eve is an artist
date: 15:05 02.01 (Mon)
message: Hi
[/MSG_ID:2]
`

	// keepUserID = 0 (общий режим)
	result := ci.StripSensitiveData(rawContext, 0)

	if strings.Contains(result, "bio:") {
		t.Errorf("В общем режиме все bio должны быть удалены, получено: %s", result)
	}
}

func TestStripSensitiveData_RemovesAtUID(t *testing.T) {
	ci := newTestContextIsolator()

	rawContext := `[MSG_ID:1]
[U600:Frank]
alias: Frank
message: Привет @U12345 как дела?
[/MSG_ID:1]
`

	result := ci.StripSensitiveData(rawContext, 0)

	if strings.Contains(result, "@U12345") {
		t.Errorf("Паттерн @U12345 должен быть удалён")
	}
	// Но alias должен сохраниться
	if !strings.Contains(result, "alias: Frank") {
		t.Errorf("Alias Frank должен сохраниться")
	}
}

func TestStripSensitiveData_RemovesUsernameInGeneralMode(t *testing.T) {
	ci := newTestContextIsolator()

	rawContext := `[MSG_ID:1]
[U700:Grace]
username: @grace123
alias: Grace
message: Test
[/MSG_ID:1]
`

	result := ci.StripSensitiveData(rawContext, 0)

	if strings.Contains(result, "username:") {
		t.Errorf("В общем режиме username должен быть удалён")
	}
	if !strings.Contains(result, "alias: Grace") {
		t.Errorf("Alias Grace должен сохраниться в общем режиме")
	}
}

func TestStripSensitiveData_RemovesRealNameInGeneralMode(t *testing.T) {
	ci := newTestContextIsolator()

	rawContext := `[MSG_ID:1]
[U800:Henry]
alias: Henry
real_name: Henry Ivanov
message: Test
[/MSG_ID:1]
`

	result := ci.StripSensitiveData(rawContext, 0)

	if strings.Contains(result, "real_name:") {
		t.Errorf("В общем режиме real_name должен быть удалён")
	}
}

func TestStripSensitiveData_GroupActiveMode(t *testing.T) {
	ci := newTestContextIsolator()

	rawContext := `[MSG_ID:1]
[U900:Ivan]
username: @ivan
alias: Ivan
bio: Ivan bio here
date: 15:04
message: Test message
[/MSG_ID:1]

[MSG_ID:2]
[U1000:Julia]
username: @julia
alias: Julia
bio: Julia bio here
date: 15:05
message: Another test
[/MSG_ID:2]
`

	// keepUserID = -1 (групповой режим: сохраняем alias, удаляем bio)
	result := ci.StripSensitiveData(rawContext, -1)

	if strings.Contains(result, "bio:") {
		t.Errorf("В групповом режиме все bio должны быть удалены")
	}
	if !strings.Contains(result, "alias: Ivan") || !strings.Contains(result, "alias: Julia") {
		t.Errorf("В групповом режиме alias должны сохраниться")
	}
}

func TestDetermineIsolationType_DirectReply(t *testing.T) {
	ci := newTestContextIsolator()

	decision := &FreeWillShouldReplyDecision{
		ReplyType: "direct_reply",
	}

	result := ci.DetermineIsolationType(0, decision, 0)
	if result != IsolationUserSpecific {
		t.Errorf("direct_reply должен давать user_specific, получено: %s", result)
	}
}

func TestDetermineIsolationType_General(t *testing.T) {
	ci := newTestContextIsolator()

	decision := &FreeWillShouldReplyDecision{
		ReplyType: "general",
	}

	result := ci.DetermineIsolationType(0, decision, 0)
	if result != IsolationGeneral {
		t.Errorf("general должен давать general, получено: %s", result)
	}
}

func TestDetermineIsolationType_ContextBased(t *testing.T) {
	ci := newTestContextIsolator()

	decision := &FreeWillShouldReplyDecision{
		ReplyType: "context_based",
	}

	result := ci.DetermineIsolationType(0, decision, 0)
	if result != IsolationGroupActive {
		t.Errorf("context_based должен давать group_active, получено: %s", result)
	}
}

func TestDetermineIsolationType_NilDecision(t *testing.T) {
	ci := newTestContextIsolator()

	result := ci.DetermineIsolationType(0, nil, 0)
	if result != IsolationGeneral {
		t.Errorf("nil decision должен давать general, получено: %s", result)
	}
}

func TestIsolateContext_UserSpecific(t *testing.T) {
	ci := newTestContextIsolator()

	rawContext := `<MESSAGE_HISTORY>
<MSG ID="1" DATE="12:00 01.01" DAY="Mon" ROLE="user">
  <USER ID="100">
    <ALIAS>Alice</ALIAS>
  </USER>
  <BIO>Alice bio</BIO>
  <TEXT>Hello</TEXT>
</MSG>
<MSG ID="2" DATE="12:01 01.01" DAY="Mon" ROLE="user">
  <USER ID="200">
    <ALIAS>Bob</ALIAS>
  </USER>
  <BIO>Bob bio</BIO>
  <TEXT>Hi</TEXT>
</MSG>
</MESSAGE_HISTORY>`

	result := ci.IsolateContext(0, 100, IsolationUserSpecific, rawContext)

	if !strings.Contains(result, "Alice bio") {
		t.Errorf("BIO целевого пользователя должно сохраниться")
	}
	if strings.Contains(result, "Bob bio") {
		t.Errorf("BIO нецелевого пользователя должно быть удалено")
	}
}

func TestIsolateContext_General(t *testing.T) {
	ci := newTestContextIsolator()

	rawContext := `<MESSAGE_HISTORY>
<MSG ID="1" DATE="12:00 01.01" DAY="Mon" ROLE="user">
  <USER ID="100">
    <USERNAME>@alice</USERNAME>
    <ALIAS>Alice</ALIAS>
  </USER>
  <BIO>Alice bio</BIO>
  <TEXT>Hello</TEXT>
</MSG>
</MESSAGE_HISTORY>`

	result := ci.IsolateContext(0, 0, IsolationGeneral, rawContext)

	if strings.Contains(result, "Alice bio") {
		t.Errorf("В общем режиме BIO должно быть удалено")
	}
	if !strings.Contains(result, "<ALIAS>Alice</ALIAS>") {
		t.Errorf("Alias должен сохраниться")
	}
}

func TestIsolateContext_EmptyContext(t *testing.T) {
	ci := newTestContextIsolator()

	result := ci.IsolateContext(0, 0, IsolationGeneral, "")
	if result != "" {
		t.Errorf("Пустой контекст должен остаться пустым")
	}
}

func TestStripSensitiveData_RemovesBioSection(t *testing.T) {
	ci := newTestContextIsolator()

	rawContext := `[BIO]Some long biography text[/BIO]
[MSG_ID:1]
[U100:Alice]
message: Hello
[/MSG_ID:1]
[AUTOBIO]Auto generated bio[/AUTOBIO]
`

	result := ci.StripSensitiveData(rawContext, 0)

	if strings.Contains(result, "[BIO]") || strings.Contains(result, "[AUTOBIO]") {
		t.Errorf("Секции [BIO] и [AUTOBIO] должны быть удалены")
	}
}

func TestStripSensitiveData_RemovesDossierSection(t *testing.T) {
	ci := newTestContextIsolator()

	rawContext := `[DOSSIER]Confidential information about user[/DOSSIER]
[MSG_ID:1]
[U100:Alice]
message: Hello
[/MSG_ID:1]
`

	result := ci.StripSensitiveData(rawContext, 0)

	if strings.Contains(result, "[DOSSIER]") {
		t.Errorf("Секция [DOSSIER] должна быть удалена")
	}
}
