package main

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	csh_auth "github.com/computersciencehouse/csh-auth"
	"github.com/computersciencehouse/vote/database"
	"github.com/computersciencehouse/vote/sse"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func main() {
	r := gin.Default()
	r.StaticFS("/static", http.Dir("static"))
	r.LoadHTMLGlob("templates/*")
	broker := sse.NewBroker()

	csh := csh_auth.CSHAuth{}
	csh.Init(
		os.Getenv("VOTE_OIDC_ID"),
		os.Getenv("VOTE_OIDC_SECRET"),
		os.Getenv("VOTE_JWT_SECRET"),
		os.Getenv("VOTE_STATE"),
		os.Getenv("VOTE_HOST"),
		os.Getenv("VOTE_HOST")+"/auth/callback",
		os.Getenv("VOTE_HOST")+"/auth/login",
		[]string{"profile", "email", "groups"},
	)

	r.GET("/auth/login", csh.AuthRequest)
	r.GET("/auth/callback", csh.AuthCallback)
	r.GET("/auth/logout", csh.AuthLogout)

	r.GET("/", csh.AuthWrapper(func(c *gin.Context) {
		cl, _ := c.Get("cshauth")
		claims := cl.(csh_auth.CSHClaims)

		if !canVote(claims.UserInfo.Groups) {
			c.HTML(403, "unauthorized.tmpl", gin.H{
				"Username": claims.UserInfo.Username,
				"FullName": claims.UserInfo.FullName,
			})
			return
		}

		polls, err := database.GetOpenPolls()
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		sort.Slice(polls, func(i, j int) bool {
			return polls[i].Id > polls[j].Id
		})

		closedPolls, err := database.GetClosedVotedPolls(claims.UserInfo.Username)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		ownedPolls, err := database.GetClosedOwnedPolls(claims.UserInfo.Username)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		closedPolls = append(closedPolls, ownedPolls...)

		sort.Slice(closedPolls, func(i, j int) bool {
			return closedPolls[i].Id > closedPolls[j].Id
		})
		closedPolls = uniquePolls(closedPolls)

		c.HTML(200, "index.tmpl", gin.H{
			"Polls":       polls,
			"ClosedPolls": closedPolls,
			"Username":    claims.UserInfo.Username,
			"FullName":    claims.UserInfo.FullName,
		})
	}))

	r.GET("/create", csh.AuthWrapper(func(c *gin.Context) {
		cl, _ := c.Get("cshauth")
		claims := cl.(csh_auth.CSHClaims)

		c.HTML(200, "create.tmpl", gin.H{
			"Username": claims.UserInfo.Username,
			"FullName": claims.UserInfo.FullName,
		})
	}))
	r.POST("/create", csh.AuthWrapper(func(c *gin.Context) {
		cl, _ := c.Get("cshauth")
		claims := cl.(csh_auth.CSHClaims)

		poll := &database.Poll{
			Id:               "",
			CreatedBy:        claims.UserInfo.Username,
			ShortDescription: c.PostForm("shortDescription"),
			LongDescription:  c.PostForm("longDescription"),
			Open:             true,
			Hidden:           false,
			AllowWriteIns:    c.PostForm("allowWriteIn") == "true",
		}

		switch c.PostForm("options") {
		case "pass-fail-conditional":
			poll.Options = []string{"Pass", "Fail/Conditional", "Abstain"}
		case "fail-conditional":
			poll.Options = []string{"Fail", "Conditional", "Abstain"}
		case "custom":
			poll.Options = []string{}
			for _, opt := range strings.Split(c.PostForm("customOptions"), ",") {
				poll.Options = append(poll.Options, strings.TrimSpace(opt))
			}
		case "pass-fail":
		default:
			poll.Options = []string{"Pass", "Fail", "Abstain"}
		}

		pollId, err := database.CreatePoll(poll)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.Redirect(302, "/poll/"+pollId)
	}))

	r.GET("/poll/:id", csh.AuthWrapper(func(c *gin.Context) {
		cl, _ := c.Get("cshauth")
		claims := cl.(csh_auth.CSHClaims)

		poll, err := database.GetPoll(c.Param("id"))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		if !poll.Open {
			c.Redirect(302, "/results/"+poll.Id)
			return
		}

		hasVoted, err := database.HasVoted(poll.Id, claims.UserInfo.Username)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if hasVoted {
			c.Redirect(302, "/results/"+poll.Id)
			return
		}

		c.HTML(200, "poll.tmpl", gin.H{
			"Id":               poll.Id,
			"ShortDescription": poll.ShortDescription,
			"LongDescription":  poll.LongDescription,
			"Options":          poll.Options,
			"AllowWriteIns":    poll.AllowWriteIns,
			"Username":         claims.UserInfo.Username,
			"FullName":         claims.UserInfo.FullName,
		})
	}))
	r.POST("/poll/:id", csh.AuthWrapper(func(c *gin.Context) {
		cl, _ := c.Get("cshauth")
		claims := cl.(csh_auth.CSHClaims)

		poll, err := database.GetPoll(c.Param("id"))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		hasVoted, err := database.HasVoted(poll.Id, claims.UserInfo.Username)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if hasVoted || !poll.Open {
			c.Redirect(302, "/results/"+poll.Id)
			return
		}

		pId, err := primitive.ObjectIDFromHex(poll.Id)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		vote := database.Vote{
			Id:     "",
			PollId: pId,
			UserId: claims.UserInfo.Username,
			Option: c.PostForm("option"),
		}

		if hasOption(poll, c.PostForm("option")) {
			vote.Option = c.PostForm("option")
		} else if poll.AllowWriteIns && c.PostForm("option") == "writein" {
			vote.Option = c.PostForm("writeinOption")
		} else {
			c.JSON(500, gin.H{"error": "Invalid Option"})
			return
		}
		database.CastVote(&vote)

		if poll, err := database.GetPoll(c.Param("id")); err == nil {
			if results, err := poll.GetResult(); err == nil {
				if bytes, err := json.Marshal(results); err == nil {
					broker.Notifier <- sse.NotificationEvent{
						EventName: poll.Id,
						Payload:   string(bytes),
					}
				}

			}
		}

		c.Redirect(302, "/results/"+poll.Id)
	}))

	r.GET("/results/:id", csh.AuthWrapper(func(c *gin.Context) {
		cl, _ := c.Get("cshauth")
		claims := cl.(csh_auth.CSHClaims)

		poll, err := database.GetPoll(c.Param("id"))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		results, err := poll.GetResult()
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.HTML(200, "result.tmpl", gin.H{
			"Id":               poll.Id,
			"ShortDescription": poll.ShortDescription,
			"LongDescription":  poll.LongDescription,
			"Results":          results,
			"IsOpen":           poll.Open,
			"IsOwner":          poll.CreatedBy == claims.UserInfo.Username,
			"Username":         claims.UserInfo.Username,
			"FullName":         claims.UserInfo.FullName,
		})
	}))

	r.POST("/poll/:id/close", csh.AuthWrapper(func(c *gin.Context) {
		cl, _ := c.Get("cshauth")
		claims := cl.(csh_auth.CSHClaims)

		poll, err := database.GetPoll(c.Param("id"))

		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		if poll.CreatedBy != claims.UserInfo.Username {
			c.JSON(403, gin.H{"error": "Only the creator can close a poll"})
			return
		}

		err = poll.Close()
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.Redirect(302, "/results/"+poll.Id)
	}))

	r.GET("/stream/:topic", csh.AuthWrapper(broker.ServeHTTP))

	go broker.Listen()

	r.Run()
}

func canVote(groups []string) bool {
	var active, fall_coop, spring_coop bool
	for _, group := range groups {
		if group == "active" {
			active = true
		}
		if group == "fall_coop" {
			fall_coop = true
		}
		if group == "spring_coop" {
			spring_coop = true
		}
	}

	if time.Now().Month() > time.July {
		return active && !fall_coop
	} else {
		return active && !spring_coop
	}
}

func uniquePolls(polls []*database.Poll) []*database.Poll {
	var unique []*database.Poll
	for _, poll := range polls {
		if !containsPoll(unique, poll) {
			unique = append(unique, poll)
		}
	}
	return unique
}

func containsPoll(polls []*database.Poll, poll *database.Poll) bool {
	for _, p := range polls {
		if p.Id == poll.Id {
			return true
		}
	}
	return false
}

func hasOption(poll *database.Poll, option string) bool {
	for _, opt := range poll.Options {
		if opt == option {
			return true
		}
	}
	return false
}
