package llm

// ResponseTypeToYAMLKey — маппинг ResponseType в YAML-ключ секции llm.response_types.
// Используется LLMRouterV2 для поиска RoutingProfile по типу ответа.
var ResponseTypeToYAMLKey = map[ResponseType]string{
	// По умолчанию
	ResponseTypeDefault: "default",

	// Основные типы
	ResponseTypeDirect:              "direct",
	ResponseTypeDirectSerious:       "direct_serious",
	ResponseTypeSerious:             "serious",
	ResponseTypeClassify:            "classify",
	ResponseTypeDailyTake:           "daily_take",
	ResponseTypeSummary:             "summary",
	ResponseTypeWeeklySummary:       "weekly_summary",
	ResponseTypeVoiceMessage:        "voice_message",
	ResponseTypeVoiceFormat:         "voice_format",
	ResponseTypeWelcome:             "welcome",
	ResponseTypePhoto:               "photo",
	ResponseTypeAutoBio:             "auto_bio",
	ResponseTypeAutoBioInitial:      "auto_bio_initial",
	ResponseTypeAutoBioUpdate:       "auto_bio_update",
	ResponseTypeSrach:               "srach",
	ResponseTypeRateLimit:           "rate_limit",
	ResponseTypeDonate:              "donate",
	ResponseTypePersonalityAnalysis: "personality_analysis",
	ResponseTypePersonalityName:     "personality_name",
	ResponseTypePersonalityTopic:    "personality_topic",
	ResponseTypePersonalitySelf:     "personality_self",
	ResponseTypeWebSearch:           "web_search",
	ResponseTypeClownReaction:       "clown_reaction",
	ResponseTypeReactionAnalysis:    "reaction_analysis",
	ResponseTypeAntiRepetition:      "anti_repetition",
	ResponseTypeModeration:          "moderation",

	// Free Will
	ResponseTypeFreeWillShouldReply:            "free_will_should_reply",
	ResponseTypeFreeWillResponseType:           "free_will_response_type",
	ResponseTypeFreeWillDirect:                 "free_will_direct",
	ResponseTypeFreeWillDirectResponseDecision: "free_will_direct_response_decision",
	ResponseTypeFreeWillDirectResponse:         "free_will_direct_response",
	ResponseTypeFreeWillGeneral:                "free_will_general",
	ResponseTypeFreeWillContext:                "free_will_context",
	ResponseTypeFreeWillSilence:                "free_will_silence",
	ResponseTypeFreeWillMoodAnalysis:           "free_will_mood_analysis",
	ResponseTypeFreeWillMoodBasedMessage:       "free_will_mood_based",
	ResponseTypeFreeWillVoiceMessage:           "free_will_voice",
	ResponseTypeFreeWillTakeResponse:           "free_will_take_response",
	ResponseTypeFreeWillReaction:               "free_will_reaction",

	// Message Post-Processor
	ResponseTypePostProcessSingleWord:  "post_process_single",
	ResponseTypePostProcessShort:       "post_process_short",
	ResponseTypePostProcessLong:        "post_process_long",
	ResponseTypePostProcessIntelligent: "post_process_intelligent",
	ResponseTypePostProcessSummary:     "post_process_summary",

	// Дополнительные
	ResponseTypePhotoAnalysis:        "photo_analysis",
	ResponseTypeWebSearchTrigger:     "web_search_trigger",
	ResponseTypeAntiRepetitionRework: "anti_repetition_rework",

	// Каузальное обучение
	ResponseTypeCausalAnalysis:  "causal_analysis",
	ResponseTypeCausalInfluence: "causal_influence",

	// Эмоциональная система
	ResponseTypeEmotionalAnalysis:   "emotional_analysis",
	ResponseTypeEmotionalAdaptation: "emotional_adaptation",
	ResponseTypeEmotionalFeedback:   "emotional_feedback",

	// Система убеждений
	ResponseTypeBeliefAnalysis: "belief_analysis",
}

// ResponseTypeToYAML возвращает YAML-ключ для заданного ResponseType.
// Если тип неизвестен — возвращает "default".
func ResponseTypeToYAML(rt ResponseType) string {
	if key, ok := ResponseTypeToYAMLKey[rt]; ok {
		return key
	}
	return "default"
}
