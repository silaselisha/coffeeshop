package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
	"github.com/silaselisha/coffee-api/internal"
	"github.com/silaselisha/coffee-api/internal/aws"
	"github.com/silaselisha/coffee-api/internal/mail"
	"github.com/silaselisha/coffee-api/pkg/store"
	"github.com/silaselisha/coffee-api/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	DefaultQueue  = "default"
	CriticalQueue = "critical"
)

type TaskProcessor interface {
	Start() error
	ProcessTaskSendVerificationMail(ctx context.Context, task *asynq.Task) error
	ProcessTaskUploadS3Object(ctx context.Context, task *asynq.Task) error
	ProcessTaskDeleteS3Object(ctx context.Context, task *asynq.Task) error
	ProcessTaskMultipleUploadS3Object(ctx context.Context, task *asynq.Task) error
}

type RedisSrvTaskProcessor struct {
	server             *asynq.Server
	store              store.Mongo
	envs               types.Config
	coffeeShopS3Bucket aws.CoffeeShopBucket
}

func NewTaskServerProcessor(opts asynq.RedisClientOpt, store store.Mongo, envs types.Config, coffeeShopS3Bucket aws.CoffeeShopBucket) TaskProcessor {

	server := asynq.NewServer(opts, asynq.Config{
		Queues: map[string]int{CriticalQueue: 1, DefaultQueue: 2},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			log.Error().Err(err).Str("type", task.Type()).Msg("process failed")
		}),
	})

	return &RedisSrvTaskProcessor{
		server:             server,
		store:              store,
		envs:               envs,
		coffeeShopS3Bucket: coffeeShopS3Bucket,
	}
}

func (processor *RedisSrvTaskProcessor) ProcessTaskSendVerificationMail(ctx context.Context, task *asynq.Task) error {
	user, err := getUserByEmail(ctx, processor, task)
	if err != nil {
		return fmt.Errorf("error occured while retreiving user %w", err)
	}

	fmt.Printf("BEGIN @%+v\n", time.Now())
	fmt.Printf("start processing task %+s\n", task.Type())

	transporter := mail.NewSMTPTransporter(&processor.envs)
	message := fmt.Sprintf("http://localhost:3000/verify?token=%s&timestamp=%d", user.Id.Hex(), internal.ResetToken(2880))

	err = transporter.MailSender(ctx, user.Email, []byte(message))
	if err != nil {
		fmt.Println(err)
		return fmt.Errorf("error occured while sending a verification mail to %s at %v err %w", user.Email, time.Now(), err)
	}

	fmt.Printf("END @%+v\n", time.Now())
	return nil
}

func (processor *RedisSrvTaskProcessor) ProcessTaskSendResetPasswordMail(ctx context.Context, task *asynq.Task) error {
	user, err := getUserByEmail(ctx, processor, task)
	if err != nil {
		return fmt.Errorf("error occured while retreiving user %w", err)
	}

	transporter := mail.NewSMTPTransporter(&processor.envs)
	message := fmt.Sprintf("http://localhost:3000/resetpassword?token=%s&timestamp=%d", user.Id.Hex(), internal.ResetToken(2880))

	err = transporter.MailSender(ctx, user.Email, []byte(message))
	if err != nil {
		return fmt.Errorf("error occured while sending a verification mail to %s at %v err %w", user.Email, time.Now(), err)
	}

	fmt.Printf("processing %s at %v\n", task.Type(), time.Now())
	return nil
}

func (processor *RedisSrvTaskProcessor) ProcessTaskUploadS3Object(ctx context.Context, task *asynq.Task) error {
	var Payload types.PayloadUploadImage
	err := json.Unmarshal(task.Payload(), &Payload)
	if err != nil {
		fmt.Print(time.Now())
		return fmt.Errorf("unmarshalling error %w", err)
	}

	fmt.Printf("BEGIN @%+v\n", time.Now())
	fmt.Printf("start processing task %+s\n", task.Type())

	err = processor.coffeeShopS3Bucket.UploadImage(ctx, Payload.ObjectKey, processor.envs.S3_BUCKET_NAME, Payload.Extension, Payload.Image)
	if err != nil {
		fmt.Print(time.Now())
		return err
	}
	fmt.Printf("END @%+v\n", time.Now())

	return nil
}

func (processor *RedisSrvTaskProcessor) ProcessTaskMultipleUploadS3Object(ctx context.Context, task *asynq.Task) error {
	var Payload []*types.PayloadUploadImage
	err := json.Unmarshal(task.Payload(), &Payload)
	if err != nil {
		fmt.Print(time.Now())
		return fmt.Errorf("unmarshalling error %w", err)
	}

	fmt.Printf("BEGIN @%+v\n", time.Now())
	fmt.Printf("start processing task %+s\n", task.Type())

	err = processor.coffeeShopS3Bucket.UploadMultipleImages(ctx, Payload, processor.envs.S3_BUCKET_NAME)
	if err != nil {
		fmt.Print(time.Now())
		return err
	}
	fmt.Printf("END @%+v\n", time.Now())

	return nil
}

func (processor *RedisSrvTaskProcessor) ProcessTaskDeleteS3Object(ctx context.Context, task *asynq.Task) error {
	var payload []string
	err := json.Unmarshal(task.Payload(), &payload)
	if err != nil {
		return fmt.Errorf("error occured while unmarshalling %w", err)
	}

	fmt.Printf("BEGIN @%+v\n", time.Now())
	fmt.Printf("start processing task %+s\n", task.Type())
	for _, image := range payload {
		err := processor.coffeeShopS3Bucket.DeleteImage(ctx, image, processor.envs.S3_BUCKET_NAME)
		if err != nil {
			fmt.Println(err)
			return err
		}
	}
	fmt.Printf("END @%+v\n", time.Now())
	return nil
}

func getUserByEmail(ctx context.Context, processor *RedisSrvTaskProcessor, task *asynq.Task) (store.User, error) {
	var Payload types.PayloadSendMail
	err := json.Unmarshal(task.Payload(), &Payload)
	if err != nil {
		fmt.Print(time.Now())
		return store.User{}, fmt.Errorf("unmarshalling error %w", err)
	}

	var user store.User
	users := processor.store.Collection(ctx, "coffeeshop", "users")
	curr := users.FindOne(ctx, bson.D{{Key: "email", Value: Payload.Email}})
	err = curr.Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			fmt.Print(time.Now())
			return store.User{}, fmt.Errorf("document not found %w", err)
		}
		return store.User{}, fmt.Errorf("internal server error %w", err)
	}

	return user, nil
}

func (processor *RedisSrvTaskProcessor) Start() error {
	mux := asynq.NewServeMux()
	mux.HandleFunc(SEND_VERIFICATION_EMAIL, processor.ProcessTaskSendVerificationMail)
	mux.HandleFunc(SEND_PASSWORD_RESET_EMAIL, processor.ProcessTaskSendResetPasswordMail)
	mux.HandleFunc(UPLOAD_S3_OBJECT, processor.ProcessTaskUploadS3Object)
	mux.HandleFunc(UPLOAD_MULTIPLE_S3_OBJECTS, processor.ProcessTaskMultipleUploadS3Object)
	mux.HandleFunc(DELETE_S3_OBJECT, processor.ProcessTaskDeleteS3Object)

	return processor.server.Start(mux)
}
