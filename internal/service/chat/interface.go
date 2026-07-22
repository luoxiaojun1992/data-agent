package chat

import (
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
)

// SessionService is the domain contract for session management, re-exported
// here as a type alias for backward compatibility with existing handler/test
// imports. The canonical definition lives in internal/domain/chat.
type SessionService = domainchat.SessionService

// Session is the domain session entity, re-exported as a type alias so
// existing references to chat.Session keep compiling. The canonical
// definition lives in internal/domain/chat.
type Session = domainchat.Session

// Message, ChatRequest, and ChatResponse are domain DTOs re-exported as
// aliases for backward compatibility.
type (
	Message      = domainchat.Message
	ChatRequest  = domainchat.ChatRequest
	ChatResponse = domainchat.ChatResponse
)
