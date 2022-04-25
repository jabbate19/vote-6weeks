package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Vote struct {
	Id     string             `bson:"_id,omitempty"`
	PollId primitive.ObjectID `bson:"pollId"`
	UserId string             `bson:"userId"`
	Option string             `bson:"option"`
}

func CastVote(vote *Vote) error {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	_, err := Client.Database("vote").Collection("votes").InsertOne(ctx, vote)
	if err != nil {
		return err
	}

	return nil
}

func HasVoted(pollId, userId string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	pId, err := primitive.ObjectIDFromHex(pollId)
	if err != nil {
		return false, err
	}

	count, err := Client.Database("vote").Collection("votes").CountDocuments(ctx, map[string]interface{}{"pollId": pId, "userId": userId})
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
