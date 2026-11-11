# The matching engine (the exchange). Built and run by docker-compose.yml.
FROM debian:bookworm-slim AS build
RUN apt-get update && apt-get install -y --no-install-recommends g++ && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY *.cpp *.hpp ./
RUN g++ -std=c++17 -O2 -o /me matching_engine.cpp

FROM debian:bookworm-slim
COPY --from=build /me /me
# Exec form: the engine is PID 1 and gets `docker stop`'s SIGTERM directly.
ENTRYPOINT ["/me", "--serve"]
CMD ["MINI"]
