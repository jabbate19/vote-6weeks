package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type RankedVote struct {
	Id      string             `bson:"_id,omitempty"`
	PollId  primitive.ObjectID `bson:"pollId"`
	UserId  string             `bson:"userId"`
	Options map[string]int     `bson:"option"`
}

func CastRankedVote(vote *RankedVote) error {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	_, err := Client.Database("vote").Collection("votes").InsertOne(ctx, vote)
	if err != nil {
		return err
	}

	return nil
}
