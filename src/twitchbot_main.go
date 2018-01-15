package main

import (
	"github.com/thoj/go-ircevent"

	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"
)

var channelFlag = flag.String("channel", "lizthegrey", "the channel to join")
var userFlag = flag.String("username", "lizthegrey", "the user to authenticate as.")
var oauthToken = flag.String("oauthToken", "", "the oauth bearer token to use.")
var allowList = flag.String("allowList", "lizthegrey,soundtom1,thedoh,notque,kelesti", "the users to allow to speak in the channel.")
var blockList = flag.String("blockList", "", "the users to ban from the channel on sight.")
var dryRun = flag.Bool("dryRun", true, "do not actually moderate chat")

type MemberList map[string]bool

var ml = make(MemberList)

var strict = false

var allowMap = make(map[string]bool)
var blockMap = make(map[string]bool)
var channel string

func main() {
	flag.Parse()
	channel = fmt.Sprintf("#%s", *channelFlag)

	ircobj := irc.IRC(*userFlag, *userFlag)
	ircobj.UseTLS = false
	ircobj.Password = fmt.Sprintf("oauth:%s", *oauthToken)
	ircobj.Connect("irc.chat.twitch.tv:6667")
	ircobj.SendRaw("CAP REQ :twitch.tv/command")
	ircobj.SendRaw("CAP REQ :twitch.tv/membership")
	ircobj.SendRaw("CAP REQ :twitch.tv/tags")

	ircobj.AddCallback("421", func(e *irc.Event) {
		fmt.Printf("Error: %s\n", e.Message())
	})

	for _, v := range strings.Split(*allowList, ",") {
		allowMap[v] = true
	}
	for _, v := range strings.Split(*blockList, ",") {
		blockMap[v] = true
	}

	ircobj.AddCallback("JOIN", JoinHandler)
	ircobj.AddCallback("PART", PartHandler)
	ircobj.AddCallback("PRIVMSG", PrivmsgHandler)

	// Wait for ourselves to become authenticated.
	time.Sleep(2 * time.Second)
	ircobj.Join(channel)

	ircobj.Privmsgf(channel, "Bot initialized. Type !bot quit to stop me.")

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		for _ = range signalChan {
			ircobj.Quit()
		}
	}()

	ircobj.Loop()
	fmt.Println("Shut down gracefully.")
}

func Timeout(ircobj *irc.Connection, user string) {
	if !*dryRun {
		ircobj.Privmsgf(channel, "/timeout %s %d", user, 60)
	} else {
		ircobj.Privmsgf(channel, "Would run /timeout %s %d", user, 60)
	}
}

func Untimeout(ircobj *irc.Connection, user string) {
	if !*dryRun {
		ircobj.Privmsgf(channel, "/untimeout %s", user)
	} else {
		ircobj.Privmsgf(channel, "Would run /untimeout %s", user)
	}
}

func JoinHandler(e *irc.Event) {
	e.Connection.Privmsgf(channel, "Saw %s joining channel. allow=%t, block=%t", e.User, allowMap[e.User], blockMap[e.User])
	ml[e.User] = true
	ApplyBans(e.Connection, e.User)
}

func PartHandler(e *irc.Event) {
	e.Connection.Privmsgf(channel, "%s left channel.", e.User)
	delete(ml, e.User)
}

func SyncBans(ircobj *irc.Connection) {
	for k, _ := range ml {
		ApplyBans(ircobj, k)
	}
}

func ApplyBans(ircobj *irc.Connection, user string) {
	if !strict {
		if !allowMap[user] && blockMap[user] {
			Timeout(ircobj, user)
		} else {
			Untimeout(ircobj, user)
		}
	} else {
		if !allowMap[user] {
			Timeout(ircobj, user)
		} else {
			Untimeout(ircobj, user)
		}
	}
}

func PrivmsgHandler(e *irc.Event) {
	if !strings.Contains(strings.ToLower(e.Message()), "!bot") {
		// This message isn't for us.
		return
	}

	exploded := strings.Split(strings.ToLower(e.Message()), " ")
	switch exploded[len(exploded)-1] {
	case "quit":
		e.Connection.Quit()
	case "allowed_only":
		strict = true
		SyncBans(e.Connection)
	case "normal":
		strict = false
		SyncBans(e.Connection)
	default:
		e.Connection.Privmsgf(channel, "Bot didn't know what to do with command '%s'", e.Message())
	}

}
