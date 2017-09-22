package twitch

import (
	"bufio"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCanCreateClient(t *testing.T) {
	client := NewClient("justinfan123123", "oauth:1123123")

	if reflect.TypeOf(*client) != reflect.TypeOf(Client{}) {
		t.Error("client is not of type Client")
	}
}

func TestCanConnectAndAuthenticate(t *testing.T) {
	var oauthMsg string
	wait := make(chan struct{})
	waitPass := make(chan struct{})
	go func() {
		ln, err := net.Listen("tcp", ":4321")
		if err != nil {
			t.Fatal(err)
		}
		close(wait)
		conn, err := ln.Accept()
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		defer conn.Close()
		for {
			message, _ := bufio.NewReader(conn).ReadString('\n')
			message = strings.Replace(message, "\r\n", "", 1)
			if strings.HasPrefix(message, "PASS") {
				oauthMsg = message
				close(waitPass)
			}
		}
	}()

	// wait for server to start
	select {
	case <-wait:
	case <-time.After(time.Second * 3):
		t.Fatal("client didn't connect")
	}

	client := NewClient("justinfan123123", "oauth:123123132")
	client.SetIrcAddress(":4321")
	go client.Connect()

	select {
	case <-waitPass:
	case <-time.After(time.Second * 3):
		t.Fatal("no oauth read")
	}

	if oauthMsg != "PASS oauth:123123132" {
		t.Fatalf("invalid authentication data: oauth: %s", oauthMsg)
	}
}

func TestCanReceivePRIVMSGMessage(t *testing.T) {
	testMessage := "@badges=subscriber/6,premium/1;color=#FF0000;display-name=Redflamingo13;emotes=;id=2a31a9df-d6ff-4840-b211-a2547c7e656e;mod=0;room-id=11148817;subscriber=1;tmi-sent-ts=1490382457309;turbo=0;user-id=78424343;user-type= :redflamingo13!redflamingo13@redflamingo13.tmi.twitch.tv PRIVMSG #pajlada :Thrashh5, FeelsWayTooAmazingMan kinda"
	wait := make(chan struct{})

	go func() {
		ln, err := net.Listen("tcp", ":4322")
		if err != nil {
			t.Fatal(err)
		}
		close(wait)
		conn, err := ln.Accept()
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		defer conn.Close()

		fmt.Fprintf(conn, "%s\r\n", testMessage)
	}()

	// wait for server to start
	select {
	case <-wait:
	case <-time.After(time.Second * 3):
		t.Fatal("client didn't connect")
	}

	client := NewClient("justinfan123123", "oauth:123123132")
	client.SetIrcAddress(":4322")
	go client.Connect()

	waitMsg := make(chan string)
	var receivedMsg string

	client.OnNewMessage(func(channel string, user User, message Message) {
		receivedMsg = message.Text
		close(waitMsg)
	})

	// wait for server to start
	select {
	case <-waitMsg:
	case <-time.After(time.Second * 3):
		t.Fatal("no message sent")
	}

	if receivedMsg != "Thrashh5, FeelsWayTooAmazingMan kinda" {
		t.Fatal("invalid message text received")
	}
}

func TestCanReceiveCLEARCHATMessage(t *testing.T) {
	testMessage := `@ban-duration=1;ban-reason=testing\sxd;room-id=11148817;target-user-id=40910607 :tmi.twitch.tv CLEARCHAT #pajlada :ampzyh`
	wait := make(chan struct{})

	go func() {
		ln, err := net.Listen("tcp", ":4323")
		if err != nil {
			t.Fatal(err)
		}
		close(wait)
		conn, err := ln.Accept()
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		defer conn.Close()

		fmt.Fprintf(conn, "%s\r\n", testMessage)
	}()

	// wait for server to start
	select {
	case <-wait:
	case <-time.After(time.Second * 3):
		t.Fatal("client didn't connect")
	}

	client := NewClient("justinfan123123", "oauth:123123132")
	client.SetIrcAddress(":4323")
	go client.Connect()

	waitMsg := make(chan string)
	var receivedMsg string

	client.OnNewClearchatMessage(func(channel string, user User, message Message) {
		receivedMsg = message.Text
		close(waitMsg)
	})

	// wait for server to start
	select {
	case <-waitMsg:
	case <-time.After(time.Second * 3):
		t.Fatal("no message sent")
	}

	assertStringsEqual(t, "ampzyh was timed out for 1s: testing xd", receivedMsg)
}

func TestCanReceiveROOMSTATEMessage(t *testing.T) {
	testMessage := `@slow=10 :tmi.twitch.tv ROOMSTATE #gempir`
	wait := make(chan struct{})

	go func() {
		ln, err := net.Listen("tcp", ":4324")
		if err != nil {
			t.Fatal(err)
		}
		close(wait)
		conn, err := ln.Accept()
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		defer conn.Close()

		fmt.Fprintf(conn, "%s\r\n", testMessage)
	}()

	// wait for server to start
	select {
	case <-wait:
	case <-time.After(time.Second * 3):
		t.Fatal("client didn't connect")
	}

	client := NewClient("justinfan123123", "oauth:123123132")
	client.SetIrcAddress(":4324")
	go client.Connect()

	waitMsg := make(chan string)
	var receivedTag string

	client.OnNewRoomstateMessage(func(channel string, user User, message Message) {
		receivedTag = message.Tags["slow"]
		close(waitMsg)
	})

	// wait for server to start
	select {
	case <-waitMsg:
	case <-time.After(time.Second * 3):
		t.Fatal("no message sent")
	}

	assertStringsEqual(t, "10", receivedTag)
}
