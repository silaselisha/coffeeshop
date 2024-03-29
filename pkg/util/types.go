package util

type SMTPTransport struct {
	Username string
	Password string
	Port     string
	Host     string
	Sender   string
}

type Config struct {
	DB_URI               string `mapstructure:"DB_URI"`
	SMTP_HOST            string `mapstructure:"SMTP_HOST"`
	SMTP_PORT            string `mapstructure:"SMTP_PORT"`
	DB_PASSWORD          string `mapstructure:"DB_PASSWORD"`
	SMTP_PASSWORD        string `mapstructure:"SMTP_PASSWORD"`
	SMTP_USERNAME        string `mapstructure:"SMTP_USERNAME"`
	S3_BUCKET_NAME       string `mapstructure:"S3_BUCKET_NAME"`
	SMTP_SENDER          string `mapstructure:"SMTP_SENDER"`
	SERVER_REST_ADDRESS  string `mapstructure:"SERVER_REST_ADDRESS"`
	SERVER_GRPC_ADDRESS  string `mapstructure:"SERVER_GRPC_ADDRESS"`
	SERVER_GRPC_GATEWAY_ADDRESS  string `mapstructure:"SERVER_GRPC_GATEWAY_ADDRESS"`
	JWT_EXPIRES_AT       string `mapstructure:"JWT_EXPIRES_AT"`
	SECRET_ACCESS_KEY    string `mapstructure:"SECRET_ACCESS_KEY"`
	REDIS_SERVER_PORT    string `mapstructure:"REDIS_SERVER_PORT"`
	REDIS_SERVER_ADDRESS string `mapstructure:"REDIS_SERVER_ADDRESS"`
}

type FileMetadata struct {
	ContetntType string
}

type PayloadUploadImage struct {
	Image     []byte `json:"image"`
	ObjectKey string `json:"objectKey"`
	Extension string `json:"extension"`
}

type PayloadSendMail struct {
	Email string `json:"email"`
}
