build:
    docker build . -t strava-hooks

start:
    docker run --rm --env-file=.env -p 8080:8080 strava-hooks
