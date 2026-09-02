package console

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/paymentorder"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// HandlePaymentMethods returns available pay methods and amount bounds.
func HandlePaymentMethods(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, err := a.Payment.Info(r.Context())
		if err != nil {
			log.Printf("console: payment methods: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load payment info"})
			return
		}
		writeJSON(w, http.StatusOK, info)
	}
}

// HandleCreatePaymentOrder places a pending recharge order and returns the pay URL.
func HandleCreatePaymentOrder(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		var req struct {
			Amount        string `json:"amount"`
			PayMethod     string `json:"pay_method"`
			PaymentMethod string `json:"payment_method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		amount, err := decimal.NewFromString(req.Amount)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid amount"})
			return
		}
		method := req.PayMethod
		if method == "" {
			method = req.PaymentMethod
		}
		if method == "" {
			method = "alipay"
		}
		res, err := a.Payment.CreateOrder(r.Context(), userID, amount, method)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// paymentOrderDTO is the user/admin order list representation.
// PayURL visibility (TH-P05-10): the pay URL can carry signed parameters, so
// it is exposed ONLY while the order is pending and ONLY to the owning user /
// admin; paid/closed/refunded orders and legacy NULL rows serialize it as
// absent (omitempty), never as a usable URL. The full URL must never be
// logged or emitted to metrics.
type paymentOrderDTO struct {
	ID        string `json:"id"`
	OrderNo   string `json:"order_no"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Channel   string `json:"channel"`
	PayMethod string `json:"pay_method"`
	Status    string `json:"status"`
	PayURL    string `json:"pay_url,omitempty"`
	CreatedAt string `json:"created_at"`
}

func toOrderDTO(o paymentorder.Order) paymentOrderDTO {
	dto := paymentOrderDTO{
		ID:        o.ID.String(),
		OrderNo:   o.OrderNo,
		Amount:    o.Amount.StringFixed(2),
		Currency:  o.Currency,
		Channel:   o.Channel,
		PayMethod: o.PayMethod,
		Status:    o.Status,
		CreatedAt: o.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if o.Status == paymentorder.StatusPending && o.PayURL != nil {
		dto.PayURL = *o.PayURL
	}
	return dto
}

// HandleListMyPaymentOrders lists the authenticated user's recharge orders.
func HandleListMyPaymentOrders(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		orders, err := a.PaymentOrders.ListByUser(r.Context(), userID, 50, 0)
		if err != nil {
			log.Printf("console: list payment orders: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load orders"})
			return
		}
		dtos := make([]paymentOrderDTO, 0, len(orders))
		for _, o := range orders {
			dtos = append(dtos, toOrderDTO(o))
		}
		writeJSON(w, http.StatusOK, map[string]any{"orders": dtos})
	}
}

// HandlePaymentNotify is the unauthenticated gateway callback (epay).
func HandlePaymentNotify(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params := map[string]string{}
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err == nil {
				for k, v := range r.PostForm {
					if len(v) > 0 {
						params[k] = v[0]
					}
				}
			}
		}
		if len(params) == 0 {
			w.Write([]byte("fail"))
			return
		}
		if _, err := a.Payment.HandleNotify(r.Context(), params); err != nil {
			log.Printf("console: payment notify: %v", err)
			w.Write([]byte("fail"))
			return
		}
		w.Write([]byte("success"))
	}
}

// HandleListAllPaymentOrders lists recharge orders for admin (with filters).
func HandleListAllPaymentOrders(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		var status *string
		if s := r.URL.Query().Get("status"); s != "" {
			status = &s
		}
		var userID *uuid.UUID
		if u := r.URL.Query().Get("user_id"); u != "" {
			if id, err := uuid.Parse(u); err == nil {
				userID = &id
			}
		}
		orders, err := a.PaymentOrders.List(r.Context(), limit, offset, status, userID)
		if err != nil {
			log.Printf("console: admin list payment orders: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load orders"})
			return
		}
		dtos := make([]paymentOrderDTO, 0, len(orders))
		for _, o := range orders {
			dtos = append(dtos, toOrderDTO(o))
		}
		writeJSON(w, http.StatusOK, map[string]any{"orders": dtos})
	}
}

// HandleAdminCompletePaymentOrder manually credits an order (callback-loss fallback).
func HandleAdminCompletePaymentOrder(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid order id"})
			return
		}
		if err := a.Payment.AdminComplete(r.Context(), id); err != nil {
			log.Printf("console: admin complete payment order: %v", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
