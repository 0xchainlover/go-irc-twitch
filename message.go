package twitch

import (
	"strconv"
	"strings"
	"time"
)

// MessageType different message types possible to receive via IRC
type MessageType int

const (
	// UNSET is the default message type, for whenever a new message type is added by twitch that we don't parse yet
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
)

type channelMessage struct {
	RawMessage
	Channel string
}

type roomMessage struct {
	channelMessage
	RoomID string
}

// Unsure of a better name, but this isn't entirely descriptive of the contents
type chatMessage struct {
	roomMessage
	ID   string
	Time time.Time
}

type userMessage struct {
	Action bool
	Emotes []*Emote
}

// Emote twitch emotes
type Emote struct {
	Name  string
	ID    string
	Count int
}

// message is purely for internal use
type message struct {
	RawMessage RawMessage
	Channel    string
	Username   string
}

func parseMessage(line string) *message {
	if !strings.HasPrefix(line, "@") {
		return &message{
			RawMessage: RawMessage{
				Type:    UNSET,
				Raw:     line,
				Message: line,
			},
		}
	}

	split := strings.SplitN(line, " :", 3)
	if len(split) < 3 {
		for i := 0; i < 3-len(split); i++ {
			split = append(split, "")
		}
	}

	rawType, channel, username := parseMiddle(split[1])

	rawMessage := RawMessage{
		Type:    parseMessageType(rawType),
		RawType: rawType,
		Raw:     line,
		Tags:    parseTags(split[0]),
		Message: split[2],
	}

	return &message{
		Channel:    channel,
		Username:   username,
		RawMessage: rawMessage,
	}
}

func parseMiddle(middle string) (string, string, string) {
	var rawType, channel, username string

	for i, v := range strings.SplitN(middle, " ", 3) {
		switch {
		case i == 1:
			rawType = v
		case strings.Contains(v, "!"):
			username = strings.SplitN(v, "!", 2)[0]
		case strings.Contains(v, "#"):
			channel = strings.TrimPrefix(v, "#")
		}
	}

	return rawType, channel, username
}

func parseMessageType(messageType string) MessageType {
	switch messageType {
	case "PRIVMSG":
		return PRIVMSG
	case "WHISPER":
		return WHISPER
	case "CLEARCHAT":
		return CLEARCHAT
	case "NOTICE":
		return NOTICE
	case "ROOMSTATE":
		return ROOMSTATE
	case "USERSTATE":
		return USERSTATE
	case "USERNOTICE":
		return USERNOTICE
	default:
		return UNSET
	}
}

func parseTags(tagsRaw string) map[string]string {
	tags := make(map[string]string)

	tagsRaw = strings.TrimPrefix(tagsRaw, "@")
	for _, v := range strings.Split(tagsRaw, ";") {
		tag := strings.SplitN(v, "=", 2)

		var value string
		if len(tag) > 1 {
			value = tag[1]
		}

		tags[tag[0]] = value
	}
	return tags
}

func (m *message) parsePRIVMSGMessage() (*User, *PRIVMSGMessage) {
	privateMessage := PRIVMSGMessage{
		chatMessage: *m.parseChatMessage(),
		userMessage: *m.parseUserMessage(),
	}

	if privateMessage.Action {
		privateMessage.Message = privateMessage.Message[8 : len(privateMessage.Message)-1]
	}

	rawBits, ok := m.RawMessage.Tags["bits"]
	if !ok {
		return m.parseUser(), &privateMessage
	}

	bits, _ := strconv.Atoi(rawBits)
	privateMessage.Bits = bits
	return m.parseUser(), &privateMessage
}

func (m *message) parseWHISPERMessage() (*User, *WHISPERMessage) {
	whisperMessage := WHISPERMessage{
		RawMessage:  m.RawMessage,
		userMessage: *m.parseUserMessage(),
	}

	if whisperMessage.Action {
		whisperMessage.Message = whisperMessage.Message[8 : len(whisperMessage.Message)-1]
	}

	return m.parseUser(), &whisperMessage
}

func (m *message) parseCLEARCHATMessage() *CLEARCHATMessage {
	clearchatMessage := CLEARCHATMessage{
		chatMessage:  *m.parseChatMessage(),
		TargetUserID: m.RawMessage.Tags["target-user-id"],
	}

	clearchatMessage.TargetUsername = clearchatMessage.Message
	clearchatMessage.Message = ""

	rawBanDuration, ok := m.RawMessage.Tags["ban-duration"]
	if !ok {
		return &clearchatMessage
	}

	banDuration, _ := strconv.Atoi(rawBanDuration)
	clearchatMessage.BanDuration = banDuration
	return &clearchatMessage
}

func (m *message) parseROOMSTATEMessage() *ROOMSTATEMessage {
	roomstateMessage := ROOMSTATEMessage{
		roomMessage: *m.parseRoomMessage(),
		Language:    m.RawMessage.Tags["broadcaster-lang"],
	}

	roomstateMessage.parseState()

	return &roomstateMessage
}

func (m *ROOMSTATEMessage) parseState() {
	m.State = make(map[string]int)

	m.addState("emote-only")
	m.addState("followers-only")
	m.addState("r9k")
	m.addState("rituals")
	m.addState("slow")
	m.addState("subs-only")
}

func (m *ROOMSTATEMessage) addState(tag string) {
	rawValue, ok := m.Tags[tag]
	if !ok {
		return
	}

	value, _ := strconv.Atoi(rawValue)
	m.State[tag] = value
}

func (m *message) parseUSERNOTICEMessage() (*User, *USERNOTICEMessage) {
	usernoticeMessage := USERNOTICEMessage{
		chatMessage: *m.parseChatMessage(),
		userMessage: *m.parseUserMessage(),
		MsgID:       m.RawMessage.Tags["msg-id"],
	}

	usernoticeMessage.parseMsgParams()

	rawSystemMsg, ok := usernoticeMessage.Tags["system-msg"]
	if !ok {
		return m.parseUser(), &usernoticeMessage
	}
	rawSystemMsg = strings.ReplaceAll(rawSystemMsg, "\\s", " ")
	rawSystemMsg = strings.ReplaceAll(rawSystemMsg, "\\n", "")
	usernoticeMessage.SystemMsg = strings.TrimSpace(rawSystemMsg)

	return m.parseUser(), &usernoticeMessage
}

func (m *USERNOTICEMessage) parseMsgParams() {
	m.MsgParams = make(map[string]interface{})

	for k, v := range m.Tags {
		if strings.Contains(k, "msg-param") {
			m.MsgParams[k] = strings.ReplaceAll(v, "\\s", " ")
		}
	}

	m.paramToInt("msg-param-cumulative-months")
	m.paramToInt("msg-param-months")
	m.paramToBool("msg-param-should-share-streak")
	m.paramToInt("msg-param-streak-months")
	m.paramToInt("msg-param-viewerCount")
}

func (m *USERNOTICEMessage) paramToBool(tag string) {
	rawValue, ok := m.MsgParams[tag]
	if !ok {
		return
	}

	m.MsgParams[tag] = rawValue.(string) == "1"
}

func (m *USERNOTICEMessage) paramToInt(tag string) {
	rawValue, ok := m.MsgParams[tag]
	if !ok {
		return
	}

	m.MsgParams[tag], _ = strconv.Atoi(rawValue.(string))
}

func (m *message) parseUSERSTATEMessage() (*User, *USERSTATEMessage) {
	userstateMessage := USERSTATEMessage{
		channelMessage: *m.parseChannelMessage(),
	}

	rawEmoteSets, ok := userstateMessage.Tags["emote-sets"]
	if !ok {
		return m.parseUser(), &userstateMessage
	}

	userstateMessage.EmoteSets = strings.Split(rawEmoteSets, ",")
	return m.parseUser(), &userstateMessage
}

func (m *message) parseNOTICEMessage() *NOTICEMessage {
	return &NOTICEMessage{
		channelMessage: *m.parseChannelMessage(),
		MsgID:          m.RawMessage.Tags["msg-id"],
	}
}

func (m *message) parseUser() *User {
	user := User{
		ID:          m.RawMessage.Tags["user-id"],
		Name:        m.Username,
		DisplayName: m.RawMessage.Tags["display-name"],
		Color:       m.RawMessage.Tags["color"],
		Badges:      m.parseBadges(),
	}

	// USERSTATE doesn't contain a Username, but it does have a display-name tag.
	if user.Name == "" {
		user.Name = strings.ToLower(user.DisplayName)
	}

	return &user
}
func (m *message) parseBadges() map[string]int {
	badges := make(map[string]int)

	rawBadges, ok := m.RawMessage.Tags["badges"]
	if !ok {
		return badges
	}

	for _, v := range strings.Split(rawBadges, ",") {
		badge := strings.SplitN(v, "/", 2)
		if len(badge) < 2 {
			continue
		}

		badges[badge[0]], _ = strconv.Atoi(badge[1])
	}

	return badges
}

func (m *message) parseChatMessage() *chatMessage {
	chatMessage := chatMessage{
		roomMessage: *m.parseRoomMessage(),
		ID:          m.RawMessage.Tags["id"],
	}

	i, err := strconv.ParseInt(m.RawMessage.Tags["tmi-sent-ts"], 10, 64)
	if err != nil {
		return &chatMessage
	}

	chatMessage.Time = time.Unix(0, int64(i*1e6))
	return &chatMessage
}

func (m *message) parseRoomMessage() *roomMessage {
	return &roomMessage{
		channelMessage: *m.parseChannelMessage(),
		RoomID:         m.RawMessage.Tags["room-id"],
	}
}

func (m *message) parseChannelMessage() *channelMessage {
	return &channelMessage{
		RawMessage: m.RawMessage,
		Channel:    m.Channel,
	}
}

func (m *message) parseUserMessage() *userMessage {
	userMessage := userMessage{
		Emotes: m.parseEmotes(),
	}

	text := m.RawMessage.Message
	if strings.HasPrefix(text, "\u0001ACTION") && strings.HasSuffix(text, "\u0001") {
		userMessage.Action = true
	}

	return &userMessage
}

func (m *message) parseEmotes() []*Emote {
	var emotes []*Emote

	rawEmotes := m.RawMessage.Tags["emotes"]
	if rawEmotes == "" {
		return emotes
	}

	runes := []rune(m.RawMessage.Message)

	for _, v := range strings.Split(rawEmotes, "/") {
		split := strings.SplitN(v, ":", 2)
		pos := strings.SplitN(split[1], ",", 2)
		indexPair := strings.SplitN(pos[0], "-", 2)
		firstIndex, _ := strconv.Atoi(indexPair[0])
		lastIndex, _ := strconv.Atoi(indexPair[1])

		e := &Emote{
			Name:  string(runes[firstIndex:lastIndex]),
			ID:    split[0],
			Count: strings.Count(split[1], ",") + 1,
		}

		emotes = append(emotes, e)
	}

	return emotes
}

func parseJoinPart(text string) (string, string) {
	username := strings.Split(text, "!")
	channel := strings.Split(username[1], "#")
	return strings.Trim(channel[1], " "), strings.Trim(username[0], " :")
}

func parseNames(text string) (string, []string) {
	lines := strings.Split(text, ":")
	channelDirty := strings.Split(lines[1], "#")
	channel := strings.Trim(channelDirty[1], " ")
	users := strings.Split(lines[2], " ")

	return channel, users
}
