package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
)

const tgDebounce = 2 * time.Second

func keychain(ctx context.Context, account string) (string, error) {
	out, err := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "tg-digest", "-a", account, "-w").Output()
	if err != nil {
		return "", fmt.Errorf("keychain tg-digest/%s: %w", account, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func tgClient(ctx context.Context, h telegram.UpdateHandler) (*telegram.Client, error) {
	var creds [3]string
	for i, a := range []string{"api_id", "api_hash", "session"} {
		v, err := keychain(ctx, a)
		if err != nil {
			return nil, err
		}
		creds[i] = v
	}
	apiID, err := strconv.Atoi(creds[0])
	if err != nil {
		return nil, fmt.Errorf("tg api_id: %w", err)
	}
	data, err := session.TelethonSession(creds[2])
	if err != nil {
		return nil, fmt.Errorf("tg session: %w", err)
	}
	store := &session.StorageMemory{}
	if err := (&session.Loader{Storage: store}).Save(ctx, data); err != nil {
		return nil, err
	}
	return telegram.NewClient(apiID, creds[1], telegram.Options{SessionStorage: store, UpdateHandler: h}), nil
}

// tgChats keeps no state on disk: dialogs are refetched on any pushed update, debounced.
func tgChats(ctx context.Context, push func([]chatEntry)) error {
	kick := make(chan struct{}, 1)
	h := telegram.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
		if tgRelevant(u, nil) {
			select {
			case kick <- struct{}{}:
			default:
			}
		}
		return nil
	})
	c, err := tgClient(ctx, h)
	if err != nil {
		return err
	}
	return c.Run(ctx, func(ctx context.Context) error {
		api := c.API()
		if _, err := api.UpdatesGetState(ctx); err != nil {
			return fmt.Errorf("tg getState: %w", err)
		}
		av := &tgAvatars{c: c, cache: map[int64][]byte{}, home: c.Config().ThisDC}
		for {
			d, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{OffsetPeer: &tg.InputPeerEmpty{}, Limit: 100})
			if err != nil {
				return fmt.Errorf("tg getDialogs: %w", err)
			}
			entries, _ := tgEntries(d, time.Now())
			for i := range entries {
				entries[i].avatar = av.get(ctx, d, entries[i].id)
			}
			push(entries)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-kick:
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(tgDebounce):
			}
		}
	})
}

func tgRelevant(u tg.UpdatesClass, muted map[string]bool) bool {
	switch u := u.(type) {
	case *tg.UpdateShortMessage:
		return !muted[tgKey(&tg.PeerUser{UserID: u.UserID})]
	case *tg.UpdateShortChatMessage:
		return !muted[tgKey(&tg.PeerChat{ChatID: u.ChatID})]
	case *tg.UpdateShort:
		return tgRelevantOne(u.Update, muted)
	case *tg.Updates:
		for _, x := range u.Updates {
			if tgRelevantOne(x, muted) {
				return true
			}
		}
	case *tg.UpdatesCombined:
		for _, x := range u.Updates {
			if tgRelevantOne(x, muted) {
				return true
			}
		}
	case *tg.UpdatesTooLong:
		return true
	}
	return false
}

func tgRelevantOne(u tg.UpdateClass, muted map[string]bool) bool {
	msgPeer := func(m tg.MessageClass) bool {
		msg, ok := m.(*tg.Message)
		return !ok || !muted[tgKey(msg.PeerID)]
	}
	switch u := u.(type) {
	case *tg.UpdateNewMessage:
		return msgPeer(u.Message)
	case *tg.UpdateNewChannelMessage:
		return msgPeer(u.Message)
	case *tg.UpdateEditMessage:
		return msgPeer(u.Message)
	case *tg.UpdateEditChannelMessage:
		return msgPeer(u.Message)
	case *tg.UpdateReadHistoryInbox, *tg.UpdateReadChannelInbox, *tg.UpdateDialogUnreadMark,
		*tg.UpdateDeleteMessages, *tg.UpdateDeleteChannelMessages, *tg.UpdateNotifySettings,
		*tg.UpdateFolderPeers, *tg.UpdateUserName:
		return true
	}
	return false
}

func tgKey(p tg.PeerClass) string {
	switch p := p.(type) {
	case *tg.PeerUser:
		return "u" + strconv.FormatInt(p.UserID, 10)
	case *tg.PeerChat:
		return "c" + strconv.FormatInt(p.ChatID, 10)
	case *tg.PeerChannel:
		return "ch" + strconv.FormatInt(p.ChannelID, 10)
	}
	return ""
}

type tgDialogs interface {
	GetDialogs() []tg.DialogClass
	GetMessages() []tg.MessageClass
	GetChats() []tg.ChatClass
	GetUsers() []tg.UserClass
}

// tgEntries maps unread dialogs; muted or archived ones are flagged muted to entries, newest first; also returns muted chat keys.
func tgEntries(d tg.MessagesDialogsClass, now time.Time) ([]chatEntry, map[string]bool) {
	ds, ok := d.(tgDialogs)
	if !ok {
		return nil, map[string]bool{}
	}
	users := map[int64]*tg.User{}
	for _, u := range ds.GetUsers() {
		if u, ok := u.(*tg.User); ok {
			users[u.ID] = u
		}
	}
	chats := map[int64]tg.ChatClass{}
	for _, c := range ds.GetChats() {
		chats[c.GetID()] = c
	}
	msgs := map[string]*tg.Message{}
	for _, m := range ds.GetMessages() {
		if m, ok := m.(*tg.Message); ok {
			msgs[tgKey(m.PeerID)+"/"+strconv.Itoa(m.ID)] = m
		}
	}
	muted := map[string]bool{}
	var out []chatEntry
	for _, dc := range ds.GetDialogs() {
		dl, ok := dc.(*tg.Dialog)
		if !ok {
			continue
		}
		key := tgKey(dl.Peer)
		mute, ok := dl.NotifySettings.GetMuteUntil()
		isMuted := ok && int64(mute) > now.Unix() || dl.FolderID == 1
		muted[key] = isMuted
		if dl.UnreadCount == 0 && !dl.UnreadMark {
			continue
		}
		e := chatEntry{app: "tg", id: key, unread: dl.UnreadCount, muted: isMuted}
		if e.unread == 0 {
			e.unread = 1
		}
		if m := msgs[key+"/"+strconv.Itoa(dl.TopMessage)]; m != nil {
			e.at = time.Unix(int64(m.Date), 0)
			e.text = tgText(m)
		}
		switch p := dl.Peer.(type) {
		case *tg.PeerUser:
			u := users[p.UserID]
			if u == nil {
				continue
			}
			e.title = strings.TrimSpace(u.FirstName + " " + u.LastName)
			if u.Username != "" {
				e.open = "tg://resolve?domain=" + u.Username
			} else {
				e.open = "tg://user?id=" + strconv.FormatInt(u.ID, 10)
			}
		case *tg.PeerChat:
			if c, ok := chats[p.ChatID].(*tg.Chat); ok {
				e.title = c.Title
			}
			e.open = "tg://openmessage?chat_id=" + strconv.FormatInt(p.ChatID, 10)
		case *tg.PeerChannel:
			if c, ok := chats[p.ChannelID].(*tg.Channel); ok {
				e.title = c.Title
				if c.Username != "" {
					e.open = "tg://resolve?domain=" + c.Username
				}
			}
			if e.open == "" {
				e.open = fmt.Sprintf("tg://privatepost?channel=%d&post=%d", p.ChannelID, dl.TopMessage)
			}
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at.After(out[j].at) })
	return out, muted
}

func tgText(m *tg.Message) string {
	t := strings.Join(strings.Fields(m.Message), " ")
	if t != "" || m.Media == nil {
		return t
	}
	switch m.Media.(type) {
	case *tg.MessageMediaPhoto:
		return "photo"
	case *tg.MessageMediaDocument:
		return "file"
	case *tg.MessageMediaGeo, *tg.MessageMediaGeoLive, *tg.MessageMediaVenue:
		return "location"
	case *tg.MessageMediaContact:
		return "contact"
	case *tg.MessageMediaPoll:
		return "poll"
	}
	return "media"
}

type tgAvatars struct {
	c     *telegram.Client
	home  int
	cache map[int64][]byte
	dcs   map[int]telegram.CloseInvoker
}

func (a *tgAvatars) get(ctx context.Context, d tg.MessagesDialogsClass, key string) []byte {
	ds := d.(tgDialogs)
	var peer tg.InputPeerClass
	var photoID int64
	var dc int
	for _, u := range ds.GetUsers() {
		if u, ok := u.(*tg.User); ok && "u"+strconv.FormatInt(u.ID, 10) == key {
			if p, ok := u.Photo.(*tg.UserProfilePhoto); ok {
				peer, photoID, dc = u.AsInputPeer(), p.PhotoID, p.DCID
			}
		}
	}
	for _, c := range ds.GetChats() {
		switch c := c.(type) {
		case *tg.Chat:
			if p, ok := c.Photo.(*tg.ChatPhoto); ok && "c"+strconv.FormatInt(c.ID, 10) == key {
				peer, photoID, dc = &tg.InputPeerChat{ChatID: c.ID}, p.PhotoID, p.DCID
			}
		case *tg.Channel:
			if p, ok := c.Photo.(*tg.ChatPhoto); ok && "ch"+strconv.FormatInt(c.ID, 10) == key {
				peer, photoID, dc = c.AsInputPeer(), p.PhotoID, p.DCID
			}
		}
	}
	if peer == nil {
		return nil
	}
	if b, ok := a.cache[photoID]; ok {
		return b
	}
	var rpc downloader.Client = a.c.API()
	if dc != a.home {
		if a.dcs == nil {
			a.dcs = map[int]telegram.CloseInvoker{}
		}
		inv, ok := a.dcs[dc]
		if !ok {
			var err error
			if inv, err = a.c.DC(ctx, dc, 1); err != nil {
				log.Printf("tg: avatar dc %d: %v", dc, err)
				return nil
			}
			a.dcs[dc] = inv
		}
		rpc = tg.NewClient(inv)
	}
	var buf bytes.Buffer
	loc := &tg.InputPeerPhotoFileLocation{Peer: peer, PhotoID: photoID}
	if _, err := downloader.NewDownloader().Download(rpc, loc).Stream(ctx, &buf); err != nil {
		log.Printf("tg: avatar %s: %v", key, err)
		return nil
	}
	a.cache[photoID] = buf.Bytes()
	return buf.Bytes()
}
