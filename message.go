package twitch

import (
	"strconv"
	"strings"
	"time"
)

// ADDING A NEW MESSAGE TYPE:
// 1. Add the message type at the bottom of the MessageType "const enum", with a unique index
// 2. Add a function at the bottom of file in the "parseXXXMessage" format, that parses your message type and returns a Message
// 3. Register the message into the map in the init function, specifying the message type value and the parser you just made

// MessageType different message types possible to receive via IRC
type MessageType int

const (
	// UNSET is for message types we currently don't support
	UNSET MessageType = -1
	// WHISPER private messages
	WHISPER MessageType = 0
	// PRIVMSG standard chat message
	PRIVMSG MessageType = 1
	// CLEARCHAT timeout messages
	CLEARCHAT MessageType = 2
	// ROOMSTATE changes like sub mode
	ROOMSTATE MessageType = 3
	// USERNOTICE messages like subs, resubs, raids, etc
	USERNOTICE MessageType = 4
	// USERSTATE messages
	USERSTATE MessageType = 5
	// NOTICE messages like sub mode, host on
	NOTICE MessageType = 6
	// JOIN whenever a user joins a channel
	JOIN MessageType = 7
	// PART whenever a user parts from a channel
	PART MessageType = 8
	// RECONNECT is sent from Twitch when they request the client to reconnect (i.e. for an irc server restart) https://dev.twitch.tv/docs/irc/commands/#reconnect-twitch-commands
	RECONNECT MessageType = 9
	// NAMES (or 353 https://www.alien.net.au/irc/irc2numerics.html#353) is the response sent from the server when the client requests a list of names for a channel
	NAMES MessageType = 10
	// PING is a message that can be sent from the IRC server. go-twitch-irc responds to PINGs automatically
	PING MessageType = 11
	// PONG is a message that should be sent from the IRC server as a response to us sending a PING message.
	PONG MessageType = 12
)

type messageTypeDescription struct {
	Type   MessageType
	Parser func(*ircMessage) Message
}

var messageTypeMap map[string]messageTypeDescription

func init() {
	messageTypeMap = map[string]messageTypeDescription{
		"WHISPER":    messageTypeDescription{WHISPER, parseWhisperMessage},
		"PRIVMSG":    messageTypeDescription{PRIVMSG, parsePrivateMessage},
		"CLEARCHAT":  messageTypeDescription{CLEARCHAT, parseClearChatMessage},
		"ROOMSTATE":  messageTypeDescription{ROOMSTATE, parseRoomStateMessage},
		"USERNOTICE": messageTypeDescription{USERNOTICE, parseUserNoticeMessage},
		"USERSTATE":  messageTypeDescription{USERSTATE, parseUserStateMessage},
		"NOTICE":     messageTypeDescription{NOTICE, parseNoticeMessage},
		"JOIN":       messageTypeDescription{JOIN, parseUserJoinMessage},
		"PART":       messageTypeDescription{PART, parseUserPartMessage},
		"RECONNECT":  messageTypeDescription{RECONNECT, parseReconnectMessage},
		"353":        messageTypeDescription{NAMES, parseNamesMessage},
		"PING":       messageTypeDescription{PING, parsePingMessage},
		"PONG":       messageTypeDescription{PONG, parsePongMessage},
	}
}

// Emote twitch emotes
type Emote struct {
	Name  string
	ID    string
	Count int
}

// ParseMessage parse a raw Twitch IRC message
func ParseMessage(line string) Message {
	ircMessage, err := parseIRCMessage(line)
	if err != nil {
		return parseRawMessage(ircMessage)
	}

	if mt, ok := messageTypeMap[ircMessage.Command]; ok {
		return mt.Parser(ircMessage)
	}

	return parseRawMessage(ircMessage)
}

func parseMessageType(messageType string) MessageType {
	if mt, ok := messageTypeMap[messageType]; ok {
		return mt.Type
	}

	return UNSET
}

func parseUser(message *ircMessage) User {
	user := User{
		ID:          message.Tags["user-id"],
		Name:        message.Source.Username,
		DisplayName: message.Tags["display-name"],
		Color:       message.Tags["color"],
		Badges:      make(map[string]int),
	}

	if rawBadges := message.Tags["badges"]; rawBadges != "" {
		user.Badges = parseBadges(rawBadges)
	}

	// USERSTATE doesn't contain a Username, but it does have a display-name tag
	if user.Name == "" && user.DisplayName != "" {
		user.Name = strings.ToLower(user.DisplayName)
		user.Name = strings.Replace(user.Name, " ", "", 1)
	}

	return user
}

func parseBadges(rawBadges string) map[string]int {
	badges := make(map[string]int)

	for _, badge := range strings.Split(rawBadges, ",") {
		pair := strings.SplitN(badge, "/", 2)
		badges[pair[0]], _ = strconv.Atoi(pair[1])
	}

	return badges
}

func parseRawMessage(message *ircMessage) *RawMessage {
	rawMessage := RawMessage{
		Raw:     message.Raw,
		Type:    parseMessageType(message.Command),
		RawType: message.Command,
		Tags:    message.Tags,
	}

	for i, v := range message.Params {
		if !strings.Contains(v, "#") {
			rawMessage.Message = strings.Join(message.Params[i:], " ")
			break
		}
	}

	return &rawMessage
}

func parseWhisperMessage(message *ircMessage) Message {
	whisperMessage := WhisperMessage{
		User: parseUser(message),

		Raw:       message.Raw,
		Type:      parseMessageType(message.Command),
		RawType:   message.Command,
		Tags:      message.Tags,
		MessageID: message.Tags["message-id"],
		ThreadID:  message.Tags["thread-id"],
	}

	if len(message.Params) == 2 {
		whisperMessage.Message = message.Params[1]
	}

	whisperMessage.Target = message.Params[0]
	whisperMessage.Emotes = parseEmotes(message.Tags["emotes"], whisperMessage.Message)

	if strings.Contains(whisperMessage.Message, "/me") {
		whisperMessage.Message = strings.TrimPrefix(whisperMessage.Message, "/me")
		whisperMessage.Action = true
	}

	return &whisperMessage
}

func parsePrivateMessage(message *ircMessage) Message {
	privateMessage := PrivateMessage{
		User: parseUser(message),

		Raw:     message.Raw,
		Type:    parseMessageType(message.Command),
		RawType: message.Command,
		Tags:    message.Tags,
		RoomID:  message.Tags["room-id"],
		ID:      message.Tags["id"],
		Time:    parseTime(message.Tags["tmi-sent-ts"]),
	}

	if len(message.Params) == 2 {
		privateMessage.Message = message.Params[1]
	}

	privateMessage.Channel = strings.TrimPrefix(message.Params[0], "#")
	privateMessage.Emotes = parseEmotes(message.Tags["emotes"], privateMessage.Message)

	rawBits, ok := message.Tags["bits"]
	if ok {
		bits, _ := strconv.Atoi(rawBits)
		privateMessage.Bits = bits
	}

	text := privateMessage.Message
	if strings.HasPrefix(text, "\u0001ACTION") && strings.HasSuffix(text, "\u0001") {
		privateMessage.Message = text[8 : len(text)-1]
		privateMessage.Action = true
	}

	return &privateMessage
}

func parseClearChatMessage(message *ircMessage) Message {
	clearChatMessage := ClearChatMessage{
		Raw:          message.Raw,
		Type:         parseMessageType(message.Command),
		RawType:      message.Command,
		Tags:         message.Tags,
		RoomID:       message.Tags["room-id"],
		Time:         parseTime(message.Tags["tmi-sent-ts"]),
		TargetUserID: message.Tags["target-user-id"],
	}

	clearChatMessage.Channel = strings.TrimPrefix(message.Params[0], "#")

	rawBanDuration, ok := message.Tags["ban-duration"]
	if ok {
		duration, _ := strconv.Atoi(rawBanDuration)
		clearChatMessage.BanDuration = duration
	}

	if len(message.Params) == 2 {
		clearChatMessage.TargetUsername = message.Params[1]
	}

	return &clearChatMessage
}

func parseRoomStateMessage(message *ircMessage) Message {
	roomStateMessage := RoomStateMessage{
		Raw:     message.Raw,
		Type:    parseMessageType(message.Command),
		RawType: message.Command,
		Tags:    message.Tags,
		RoomID:  message.Tags["room-id"],
		State:   make(map[string]int),
	}

	roomStateMessage.Channel = strings.TrimPrefix(message.Params[0], "#")

	stateTags := []string{"emote-only", "followers-only", "r9k", "rituals", "slow", "subs-only"}
	for _, tag := range stateTags {
		rawValue, ok := message.Tags[tag]
		if !ok {
			continue
		}

		value, _ := strconv.Atoi(rawValue)
		roomStateMessage.State[tag] = value
	}

	return &roomStateMessage
}

func parseUserNoticeMessage(message *ircMessage) Message {
	userNoticeMessage := UserNoticeMessage{
		User: parseUser(message),

		Raw:       message.Raw,
		Type:      parseMessageType(message.Command),
		RawType:   message.Command,
		Tags:      message.Tags,
		RoomID:    message.Tags["room-id"],
		ID:        message.Tags["id"],
		Time:      parseTime(message.Tags["tmi-sent-ts"]),
		MsgID:     message.Tags["msg-id"],
		MsgParams: make(map[string]string),
		SystemMsg: message.Tags["system-msg"],
	}

	if len(message.Params) == 2 {
		userNoticeMessage.Message = message.Params[1]
	}

	userNoticeMessage.Channel = strings.TrimPrefix(message.Params[0], "#")
	userNoticeMessage.Emotes = parseEmotes(message.Tags["emotes"], userNoticeMessage.Message)

	for tag, value := range message.Tags {
		if strings.Contains(tag, "msg-param") {
			userNoticeMessage.MsgParams[tag] = value
		}
	}

	return &userNoticeMessage
}

func parseUserStateMessage(message *ircMessage) Message {
	userStateMessage := UserStateMessage{
		User: parseUser(message),

		Raw:     message.Raw,
		Type:    parseMessageType(message.Command),
		RawType: message.Command,
		Tags:    message.Tags,
	}

	userStateMessage.Channel = strings.TrimPrefix(message.Params[0], "#")

	rawEmoteSets, ok := message.Tags["emote-sets"]
	if ok {
		userStateMessage.EmoteSets = strings.Split(rawEmoteSets, ",")
	}

	return &userStateMessage
}

func parseNoticeMessage(message *ircMessage) Message {
	noticeMessage := NoticeMessage{
		Raw:     message.Raw,
		Type:    parseMessageType(message.Command),
		RawType: message.Command,
		Tags:    message.Tags,
		MsgID:   message.Tags["msg-id"],
	}

	if len(message.Params) == 2 {
		noticeMessage.Message = message.Params[1]
	}

	noticeMessage.Channel = strings.TrimPrefix(message.Params[0], "#")

	return &noticeMessage
}

func parseUserJoinMessage(message *ircMessage) Message {
	parsedMessage := UserJoinMessage{
		Raw:     message.Raw,
		Type:    parseMessageType(message.Command),
		RawType: message.Command,

		User: message.Source.Username,
	}

	if len(message.Params) == 1 {
		parsedMessage.Channel = strings.TrimPrefix(message.Params[0], "#")
	}

	return &parsedMessage
}

func parseUserPartMessage(message *ircMessage) Message {
	parsedMessage := UserPartMessage{
		Raw:     message.Raw,
		Type:    parseMessageType(message.Command),
		RawType: message.Command,

		User: message.Source.Username,
	}

	if len(message.Params) == 1 {
		parsedMessage.Channel = strings.TrimPrefix(message.Params[0], "#")
	}

	return &parsedMessage
}

func parseReconnectMessage(message *ircMessage) Message {
	return &ReconnectMessage{
		Raw:     message.Raw,
		Type:    parseMessageType(message.Command),
		RawType: message.Command,
	}
}

func parseNamesMessage(message *ircMessage) Message {
	parsedMessage := NamesMessage{
		Raw:     message.Raw,
		Type:    parseMessageType(message.Command),
		RawType: message.Command,
	}

	if len(message.Params) == 4 {
		parsedMessage.Channel = strings.TrimPrefix(message.Params[2], "#")
		parsedMessage.Users = strings.Split(message.Params[3], " ")
	}

	return &parsedMessage
}

func parsePingMessage(message *ircMessage) Message {
	parsedMessage := PingMessage{
		Raw:     message.Raw,
		Type:    parseMessageType(message.Command),
		RawType: message.Command,
	}

	if len(message.Params) == 1 {
		parsedMessage.Message = strings.Split(message.Params[0], " ")[0]
	}

	return &parsedMessage
}

func parsePongMessage(message *ircMessage) Message {
	parsedMessage := PongMessage{
		Raw:     message.Raw,
		Type:    parseMessageType(message.Command),
		RawType: message.Command,
	}

	if len(message.Params) == 2 {
		parsedMessage.Message = strings.Split(message.Params[1], " ")[0]
	}

	return &parsedMessage
}

func parseTime(rawTime string) time.Time {
	if rawTime == "" {
		return time.Time{}
	}

	time64, _ := strconv.ParseInt(rawTime, 10, 64)
	return time.Unix(0, int64(time64*1e6))
}

func parseEmotes(rawEmotes, message string) []*Emote {
	var emotes []*Emote

	if rawEmotes == "" {
		return emotes
	}

	runes := []rune(message)

	for _, v := range strings.Split(rawEmotes, "/") {
		split := strings.SplitN(v, ":", 2)
		pairs := strings.SplitN(split[1], ",", 2)
		pair := strings.SplitN(pairs[0], "-", 2)

		firstIndex, _ := strconv.Atoi(pair[0])
		lastIndex, _ := strconv.Atoi(pair[1])

		emote := &Emote{
			Name:  string(runes[firstIndex : lastIndex+1]),
			ID:    split[0],
			Count: strings.Count(split[1], ",") + 1,
		}

		emotes = append(emotes, emote)
	}

	return emotes
}
