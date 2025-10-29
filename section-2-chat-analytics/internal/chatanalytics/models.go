package chatanalytics

import "time"

type Message struct {
	BotID     string    `json:"bot_id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type ProcessedMessage struct {
	Message
	NLPSentiment string  `json:"nlp_sentiment"`
	NLPScore     float64 `json:"nlp_score"`
}
