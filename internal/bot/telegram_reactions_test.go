package bot

import (
	"testing"
)

func TestReactionEncoding(t *testing.T) {
	api := NewTelegramReactionsAPI("test_token", false)

	// Тест кодирования реакций
	reactions := []ReactionType{
		{Type: "emoji", Emoji: "👍"},
		{Type: "emoji", Emoji: "❤️"},
	}

	encoded := api.encodeReactions(reactions)
	expected := "👍,❤️"
	if encoded != expected {
		t.Errorf("Ожидалось %s, получено %s", expected, encoded)
	}

	// Тест пустых реакций
	emptyEncoded := api.encodeReactions([]ReactionType{})
	if emptyEncoded != "none" {
		t.Errorf("Ожидалось 'none', получено %s", emptyEncoded)
	}
}

func TestReactionDecoding(t *testing.T) {
	api := NewTelegramReactionsAPI("test_token", false)

	// Тест декодирования реакций
	data := "reaction:👍,❤️:🤡"
	oldReactions, newReactions, isReaction := api.DecodeReactionData(data)

	if !isReaction {
		t.Error("Должно быть определено как реакция")
	}

	if len(oldReactions) != 2 || oldReactions[0] != "👍" || oldReactions[1] != "❤️" {
		t.Errorf("Неверные старые реакции: %v", oldReactions)
	}

	if len(newReactions) != 1 || newReactions[0] != "🤡" {
		t.Errorf("Неверные новые реакции: %v", newReactions)
	}

	// Тест с пустыми реакциями
	data2 := "reaction:none:👍"
	oldReactions2, newReactions2, isReaction2 := api.DecodeReactionData(data2)

	if !isReaction2 {
		t.Error("Должно быть определено как реакция")
	}

	if len(oldReactions2) != 0 {
		t.Errorf("Старые реакции должны быть пустыми: %v", oldReactions2)
	}

	if len(newReactions2) != 1 || newReactions2[0] != "👍" {
		t.Errorf("Неверные новые реакции: %v", newReactions2)
	}

	// Тест не-реакции
	data3 := "settings:some_data"
	_, _, isReaction3 := api.DecodeReactionData(data3)

	if isReaction3 {
		t.Error("Не должно быть определено как реакция")
	}
}
