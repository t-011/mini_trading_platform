DO NOT WRITE CODE IN THIS REPOSITORY.

GIVE HIM HINTS AND EXPLANATIONS IF HE NEEDS ANY. AND IF HE DOES ASK YOU, YOU HAVE TO QUIZ HIM AFTERWARDS.

HE ALSO HAS TO WRITE DOWN IMPORTANT ADRs.

ULTIMATE GOAL:
The project: a mini trading platform

1. Matching engine (the impressive core)

Users place buy and sell orders, and your engine matches them, like a real exchange.
Built on an order book: heaps or sorted trees, so your DSA work pays off directly.
Hard part: correct and fast. Measure orders per second and latency.

2. Backtesting service

Users write a strategy (e.g., “buy when the 20-day average crosses the 50-day”) and test it on past price data.
Show returns, drawdown, and Sharpe ratio.
CPU-heavy and bursty, so it runs on queued workers that scale on their own.

3. Market data service

Pulls in live or historical prices and streams updates to users over WebSockets.

4. Web app

Charts, order entry, portfolio view, strategy results.


CURRENT GOAL:
Here are the core modules, with hints only. Work out the details yourself.

1. Auth (users)

Sign up, log in, log out.
Hint: never store passwords as plain text. Look up bcrypt.
Think: sessions or JWT? What happens when a token expires?

2. Market data (prices)

Get current and past prices for a stock.
Hint: price APIs have rate limits. How could you avoid calling them every time someone loads a page?
Think: what if the price API is down?

3. Trading (fake money)

Buy and sell at the current price, and update cash and holdings.
Hint: remember the “no floats for money” rule.
Think: what if two requests try to spend the same cash at once? Look up database transactions.

4. Portfolio

Show holdings, total value, and profit/loss.
Think: do you store the value, or calculate it from trades every time? What are the trade-offs?

5. Journal

Each trade has a reason, tags, and optional notes added later.
Think: is the journal entry part of the trade, or its own table? Why?

6. Review / insights

Stats like win rate by tag, average gain, best and worst trades.
Hint: try writing this in SQL first, before Go.
Think: which numbers actually help someone learn?