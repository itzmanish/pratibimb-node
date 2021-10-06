package utils

import (
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/itzmanish/go-micro/v2/logger"
	"golang.org/x/crypto/argon2"
)

const DefaultFormat string = "01-11-2000"

type PasswordConfig struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
}

var DefaultPasswordConfig *PasswordConfig = &PasswordConfig{
	time:    1,
	memory:  64 * 1024,
	threads: 4,
	keyLen:  32,
}

// StringToTime converts 02-11-2011 to time.Time
func StringToTimeString(s string) (string, error) {
	arr := strings.Split(s, "-")
	var arrInt []int
	for _, v := range arr {
		i, err := strconv.Atoi(v)
		if err != nil {
			return time.Time{}.String(), err
		}
		arrInt = append(arrInt, i)
	}
	loc, _ := time.LoadLocation("Asia/Kolkata")

	t := time.Date(arrInt[2], time.Month(arrInt[1]), arrInt[0], 0, 0, 0, 0, loc).String()
	logger.Info(t)
	// t, err := time.Parse("2006-01-02 3:04PM", t.String())
	// if err != nil {
	// 	return time.Time{}, err
	// }

	return t, nil
}

// TimeToString converts time.Time to 02-11-2011
func TimeToString(t time.Time, format string) string {

	if format != "" {
		return t.Format(format)
	}
	return t.Format(DefaultFormat)

}

func GetKey(path string) string {
	privateKeyFile, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	return base64.StdEncoding.EncodeToString(privateKeyFile)
}

// GeneratePassword is used to generate a new password hash for storing and
// comparing at a later date.
func GeneratePassword(password string) (string, error) {
	// Generate a Salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, DefaultPasswordConfig.time, DefaultPasswordConfig.memory, DefaultPasswordConfig.threads, DefaultPasswordConfig.keyLen)

	// Base64 encode the salt and hashed password.
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	format := "$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s"
	full := fmt.Sprintf(format, argon2.Version, DefaultPasswordConfig.memory, DefaultPasswordConfig.time, DefaultPasswordConfig.threads, b64Salt, b64Hash)
	return full, nil
}

// ComparePassword is used to compare a user-inputted password to a hash to see
// if the password matches or not.
func ComparePassword(password, hash string) (bool, error) {

	parts := strings.Split(hash, "$")

	c := &PasswordConfig{}
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &c.memory, &c.time, &c.threads)
	if err != nil {
		return false, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}

	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	c.keyLen = uint32(len(decodedHash))

	comparisonHash := argon2.IDKey([]byte(password), salt, c.time, c.memory, c.threads, c.keyLen)

	return (subtle.ConstantTimeCompare(decodedHash, comparisonHash) == 1), nil
}

// LoadTLSCredentials return *tls.Config with loaded credentials
func LoadTLSCredentials(PublicCertPath, PrivateCertPath string) *tls.Config {
	// Load server's certificate and private key
	serverCert, err := tls.LoadX509KeyPair(PublicCertPath, PrivateCertPath)
	if err != nil {
		log.Fatalf(fmt.Sprintf("error on loading tls certificates: %v", err))
	}

	// disable "G402 (CWE-295): TLS MinVersion too low. (Confidence: HIGH, Severity: HIGH)"
	// #nosec G402
	// Create the credentials and return it
	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.NoClientCert,
		// RootCAs:      certPool,
	}
	return config
}
