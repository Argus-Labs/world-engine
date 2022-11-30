package argus

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/abci/types"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
)

const SockAddr = "/tmp/echo.sock"

const (
	queryType = "query"
	txType    = "tx"
)

type Request struct {
	// Typ is the type of the request. It can either be "tx" or "query"
	Typ string `json:"typ"`
	// Route is only used when Typ is a query type. It is the route of the GRPC Query Handler.
	Route string `json:"route"`
	// Payload is a json encoded string of the payload. If Typ is tx, Payload should be a json encoded sdk.Msg string.
	// If Typ is query, Payload should be a json encoded RequestQuery string.
	Payload string `json:"payload"`
}

func handleRequest(c net.Conn, app *ArgusApp) {
	app.Logger().Info("client connected ", c.RemoteAddr().Network())

	ctx := sdk.NewContext(app.GetBaseApp().CommitMultiStore(), tmproto.Header{}, false, app.Logger())
	//err := app.BankKeeper.MintCoins(ctx, "sidecar", sdk.Coins{sdk.NewInt64Coin("argus", 15)})
	//if err != nil {
	//	app.Logger().Error(err.Error())
	//}
	bz, err := io.ReadAll(c)
	if err != nil {
		app.Logger().Error("error reading data from UDS", "error", err.Error())
	}
	app.Logger().Info("DATA RECEIVED", "data", string(bz))
	c.Close()
	req := Request{}
	err = json.Unmarshal(bz, &req)
	if err != nil {
		app.Logger().Error("error unmarshalling request", "error", err.Error())
	}
	app.Logger().Info(fmt.Sprintf("got request: %v", req))

	switch req.Typ {
	case txType:
		var msg sdk.Msg
		err := app.appCodec.UnmarshalJSON([]byte(req.Payload), msg)
		if err != nil {
			// handle error
			app.Logger().Error("error unmarshalling payload into sdk.Msg", "error", err.Error())
		}
		handler := app.MsgServiceRouter().Handler(msg)
		result, err := handler(ctx, msg)
		if err != nil {
			// handle error
			app.Logger().Error("error in msg handler", "error", err.Error())
		}
		// do something with result
		_ = result
	case queryType:
		var qr types.RequestQuery
		err := json.Unmarshal([]byte(req.Payload), &qr)
		if err != nil {
			// handle err
			app.Logger().Error("error unmarshalling payload into RequestQuery", "error", err.Error())
		}
		handler := app.GRPCQueryRouter().Route(req.Route)
		resp, err := handler(ctx, qr)
		if err != nil {
			// handle err
			app.Logger().Error("error handling GRPCQuery", "error", err.Error())
		}
		// do something with result
		_ = resp
	}
}

func (app *ArgusApp) StartSidecar() {
	app.Logger().Info("Sidecar process started")
	if err := os.RemoveAll(SockAddr); err != nil {
		app.Logger().Error(err.Error())
	}

	go func() {
		l, err := net.Listen("unix", SockAddr)
		if err != nil {
			app.Logger().Error("listen error: %s", err.Error())
		}
		defer l.Close()

		for {
			// Accept new connections, dispatching them to a new goroutine.
			conn, err := l.Accept()
			if err != nil {
				app.Logger().Error("accept error: ", err.Error())
			}

			go handleRequest(conn, app)
		}
	}()
}
