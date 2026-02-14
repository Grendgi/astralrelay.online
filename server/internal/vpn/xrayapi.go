package vpn

import (
	"context"
	"fmt"
	"time"

	"github.com/xtls/xray-core/app/proxyman/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"
	"github.com/xtls/xray-core/proxy/vmess"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	inboundTagVMess  = "vmess-in"
	inboundTagVLESS  = "vless-in"
	inboundTagTrojan = "trojan-in"
)

// XrayAPI calls Xray gRPC API to add/remove users. Nil-safe: no-op if addr is empty.
type XrayAPI struct {
	addr   string
	conn   *grpc.ClientConn
	client command.HandlerServiceClient
}

// NewXrayAPI creates a client. addr empty = no-op (AddUser/RemoveUser do nothing).
func NewXrayAPI(addr string) (*XrayAPI, error) {
	if addr == "" {
		return &XrayAPI{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("xray api dial: %w", err)
	}
	return &XrayAPI{
		addr:   addr,
		conn:   conn,
		client: command.NewHandlerServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection.
func (a *XrayAPI) Close() error {
	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}

// AddVMessUser adds a VMess user to the inbound.
func (a *XrayAPI) AddVMessUser(ctx context.Context, uuid, email string) error {
	if a.client == nil {
		return nil
	}
	user := &protocol.User{
		Email: email,
		Account: serial.ToTypedMessage(&vmess.Account{
			Id:      uuid,
			AlterId: 0,
		}),
	}
	op := serial.ToTypedMessage(&command.AddUserOperation{User: user})
	_, err := a.client.AlterInbound(ctx, &command.AlterInboundRequest{
		Tag:       inboundTagVMess,
		Operation: op,
	})
	return err
}

// AddVLESSUser adds a VLESS user.
func (a *XrayAPI) AddVLESSUser(ctx context.Context, uuid, email string) error {
	if a.client == nil {
		return nil
	}
	user := &protocol.User{
		Email: email,
		Account: serial.ToTypedMessage(&vless.Account{
			Id:         uuid,
			Flow:       "xtls-rprx-vision",
			Encryption: "none",
		}),
	}
	op := serial.ToTypedMessage(&command.AddUserOperation{User: user})
	_, err := a.client.AlterInbound(ctx, &command.AlterInboundRequest{
		Tag:       inboundTagVLESS,
		Operation: op,
	})
	return err
}

// AddTrojanUser adds a Trojan user.
func (a *XrayAPI) AddTrojanUser(ctx context.Context, password, email string) error {
	if a.client == nil {
		return nil
	}
	user := &protocol.User{
		Email: email,
		Account: serial.ToTypedMessage(&trojan.Account{
			Password: password,
		}),
	}
	op := serial.ToTypedMessage(&command.AddUserOperation{User: user})
	_, err := a.client.AlterInbound(ctx, &command.AlterInboundRequest{
		Tag:       inboundTagTrojan,
		Operation: op,
	})
	return err
}

// RemoveUser removes a user by email from the given inbound.
func (a *XrayAPI) RemoveUser(ctx context.Context, tag, email string) error {
	if a.client == nil {
		return nil
	}
	op := serial.ToTypedMessage(&command.RemoveUserOperation{Email: email})
	_, err := a.client.AlterInbound(ctx, &command.AlterInboundRequest{
		Tag:       tag,
		Operation: op,
	})
	return err
}
