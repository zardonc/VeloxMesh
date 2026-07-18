package bff

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type MailSender interface {
	Send(to string, subject string, body string) error
}

type SMTPConfig struct {
	Host       string
	Port       string
	Username   string
	Password   string
	From       string
	TLSMode    string
	ServerName string
	Timeout    time.Duration
}

type smtpMailSender struct {
	config SMTPConfig
}

func newSMTPMailSender(config SMTPConfig) (*smtpMailSender, error) {
	config.Host = strings.TrimSpace(config.Host)
	config.Port = strings.TrimSpace(config.Port)
	config.Username = strings.TrimSpace(config.Username)
	config.From = strings.TrimSpace(config.From)
	config.TLSMode = strings.ToLower(strings.TrimSpace(config.TLSMode))
	config.ServerName = strings.TrimSpace(config.ServerName)
	if config.Port == "" {
		config.Port = "587"
	}
	if config.TLSMode == "" {
		config.TLSMode = "starttls"
	}
	if config.ServerName == "" {
		config.ServerName = config.Host
	}
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	if config.Host == "" || config.Username == "" || config.Password == "" || config.From == "" {
		return nil, errors.New("SMTP configuration is incomplete")
	}
	if config.TLSMode != "starttls" && config.TLSMode != "tls" {
		return nil, errors.New("SMTP TLS mode must be starttls or tls")
	}
	return &smtpMailSender{config: config}, nil
}

func (sender *smtpMailSender) tlsConfig() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, ServerName: sender.config.ServerName}
}

func (sender *smtpMailSender) Send(to string, subject string, body string) error {
	address := net.JoinHostPort(sender.config.Host, sender.config.Port)
	dialer := &net.Dialer{Timeout: sender.config.Timeout}
	client, err := sender.connect(dialer, address)
	if err != nil {
		return err
	}
	defer client.Close()

	auth := smtp.PlainAuth("", sender.config.Username, sender.config.Password, sender.config.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}
	if err := client.Mail(sender.config.From); err != nil {
		return fmt.Errorf("SMTP sender rejected: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP recipient rejected: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP data command failed: %w", err)
	}
	message := strings.Join([]string{
		"From: " + sender.config.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("SMTP message write failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("SMTP message completion failed: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("SMTP quit failed: %w", err)
	}
	return nil
}

func (sender *smtpMailSender) connect(dialer *net.Dialer, address string) (*smtp.Client, error) {
	if sender.config.TLSMode == "tls" {
		connection, err := tls.DialWithDialer(dialer, "tcp", address, sender.tlsConfig())
		if err != nil {
			return nil, fmt.Errorf("SMTP TLS connection failed: %w", err)
		}
		client, err := smtp.NewClient(connection, sender.config.Host)
		if err != nil {
			_ = connection.Close()
			return nil, fmt.Errorf("SMTP client setup failed: %w", err)
		}
		return client, nil
	}

	connection, err := dialer.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("SMTP connection failed: %w", err)
	}
	client, err := smtp.NewClient(connection, sender.config.Host)
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("SMTP client setup failed: %w", err)
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		_ = client.Close()
		return nil, errors.New("SMTP server does not support required STARTTLS")
	}
	if err := client.StartTLS(sender.tlsConfig()); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("SMTP STARTTLS failed: %w", err)
	}
	return client, nil
}
