# API

Base URL under Docker Compose: `http://localhost:8099`. Request and response bodies are JSON. Routes marked **auth** need `Authorization: Bearer <token>`, using the `Token` from login. Access tokens expire; get a new one from `/v1/refresh`.

Money is always an integer number of cents. Quantity on orders is a string of whole shares.

## Users

| Method | Path | Body | Returns |
|---|---|---|---|
| POST | `/v1/users` | `{"Email", "Password"}` | 201, the user. Also opens a paper-cash account with $100,000. |
| PUT | `/v1/users` **auth** | `{"Email", "Password"}` | 200, the updated user |
| POST | `/v1/login` | `{"Email", "Password"}` | 200, the user plus `Token` and `RefreshToken` |
| POST | `/v1/refresh` | none; send `Bearer <RefreshToken>` | 200, `{"Token"}` |
| POST | `/v1/revoke` | none; send `Bearer <RefreshToken>` | 204 |

## Stocks

These routes are public.

| Method | Path | Query | Returns |
|---|---|---|---|
| GET | `/v1/stocks` | `search`, `limit`, `offset` | `{"Stocks": [...]}` |
| GET | `/v1/stocks/{id}` | | `{"Stock": {...}}` |
| GET | `/v1/stocks/{id}/bars` | `interval` (`day`, `week`, `month`), `from`, `to` (`YYYY-MM-DD`) | `{"Bars": [...]}`, the last 30 days by default |

`limit` defaults to 50 and is clamped between 10 and 100.

## Orders

All order routes need auth. Only stocks with `Exchange = "MINI_EXCHANGE"` accept orders.

```http
POST /v1/orders
{"StockID": "<uuid>", "Side": "buy", "Quantity": "10", "Price": 10000}
```

`Price` is the limit in cents. The response is 201 with the order and any fills it got right away:

```json
{"Order": {"ID": 1, "Side": "buy", "Price": 10000, "Qty": 10, "Filled": 0, "Canceled": 0, "Status": "open", ...},
 "Trades": []}
```

A buy reserves `Price × Quantity` from cash when you place it. Fills below the limit refund the difference, and a cancel refunds the rest.

| Method | Path | Returns |
|---|---|---|
| GET | `/v1/orders?limit=&offset=` | `{"Orders": [...]}`, newest first |
| DELETE | `/v1/orders/{id}` | `{"Order": {...}}` with `"Status": "canceled"` |

Order status is one of `open`, `filled`, `canceled` or `rejected`.

## Trades, portfolio, cash

All need auth.

| Method | Path | Body | Returns |
|---|---|---|---|
| GET | `/v1/trades?limit=&offset=` | | `{"Trades": [...]}`, your fills, newest first |
| GET | `/v1/trades/{id}/notes` | | `{"Notes": [...]}` |
| POST | `/v1/trades/{id}/notes` | `{"Text"}` | 201, `{"Note": {...}}` |
| GET | `/v1/portfolio` | | `{"Cash", "Holdings": [...]}` |
| GET | `/v1/cash` | | `{"Cash"}` |

Each holding has the quantity, the average buy price, the last price, market value, cost basis and unrealized profit. The average cost is a weighted average of all buys, not first-in-first-out lots.

## Errors

Errors return `{"Error": "<message>"}`.

| Status | When |
|---|---|
| 400 | bad body or field: side, quantity, price, interval, empty note, missing email or password |
| 401 | missing or invalid token, wrong email or password |
| 404 | stock, order or trade not found, or it belongs to another user |
| 409 | email already registered; cancelling an order that isn't open |
| 422 | not enough cash or shares; the stock is read-only |
| 503 | the exchange is down |
| 500 | anything else; the server logs it |

JSON field names are the Go struct field names (`ID`, `StockID`, `CreatedAt`). Query parameters are lowercase.
