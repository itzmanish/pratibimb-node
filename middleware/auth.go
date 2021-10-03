package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/textproto"
	"time"

	"github.com/itzmanish/go-micro/v2/client"
	"github.com/itzmanish/go-micro/v2/errors"
	"github.com/itzmanish/go-micro/v2/metadata"
	"github.com/itzmanish/go-micro/v2/util/ctx"
	authpb "github.com/itzmanish/pratibimb-go/auth/proto/auth/v1"
)

const TokenCookieName = "micro-token"

// Wrapper wraps a handler and authenticates requests
func AuthWrapper(c client.Client) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return authWrapper{
			handler: h,
			auth:    authpb.NewAuthService("com.itzmanish.pratibimb.service.v1.auth", c),
		}
	}
}

type authWrapper struct {
	handler http.Handler
	auth    authpb.AuthService
}

func (h authWrapper) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if req.RequestURI == "/" {
		h.ServeHTTP(w, req)
		return
	}

	// Extract the token from the request
	token, err := getURLToken(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// if header := req.Header.Get("Authorization"); len(header) > 0 {
	// 	// Extract the auth token from the request
	// 	if strings.HasPrefix(header, auth.BearerScheme) {
	// 		token = header[len(auth.BearerScheme):]
	// 	}
	// } else {
	// 	// Get the token out the cookies if not provided in headers
	// 	if c, err := req.Cookie("micro-token"); err == nil && c != nil {
	// 		token = strings.TrimPrefix(c.Value, TokenCookieName+"=")
	// 		req.Header.Set("Authorization", auth.BearerScheme+token)
	// 	}
	// }

	var duration = time.Second * 5
	if t := req.Header.Get("Timeout"); t != "" {
		// assume timeout integer seconds now
		if td, err := time.ParseDuration(t); err == nil {
			duration = td
		}
	}

	ctxt, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	// Get the account using the token, some are unauthenticated, so the lack of an
	// account doesn't necesserially mean a forbidden request
	tokenInfo, err := h.auth.VerifyToken(ctxt, &authpb.Token{AccessToken: token})
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if tokenInfo != nil && tokenInfo.UserId != "" {
		var mtd map[string]interface{}
		err = json.Unmarshal([]byte(tokenInfo.GetMetadata()), &mtd)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if mtd["type"] == "refresh" {
			http.Error(w, "This doesn't seems like access token!", http.StatusForbidden)
			return
		}
		req.Header.Set("UserID", tokenInfo.UserId)
		// create context
		cx := ctx.FromRequest(req)
		// get context from http handler wrappers
		md, ok := metadata.FromContext(req.Context())
		if !ok {
			md = make(metadata.Metadata)
		}

		for k, _ := range req.Header {
			// may be need to get all values for key like r.Header.Values() provide in go 1.14
			md[textproto.CanonicalMIMEHeaderKey(k)] = req.Header.Get(k)
		}

		// merge context with overwrite
		cx = metadata.MergeContext(cx, md, true)

		// set merged context to request
		*req = *req.Clone(cx)
		h.handler.ServeHTTP(w, req)
		return
	}

}

func getURLToken(r *http.Request) (string, error) {
	query := r.URL.Query()
	t := query.Get("token")
	if t == "" {
		return "", errors.Unauthorized("Api.Auth", "Auth token is required.")
	}
	// cleanup from query
	query.Del("token")
	r.URL.RawQuery = query.Encode()
	return t, nil
}
