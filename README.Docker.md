### Building and running your application

When you're ready, start your application by running:
`docker compose up --build`.

Your application will be available at https://localhost (Caddy issues itself
a self-signed certificate for local use, so expect a browser warning on
first visit -- see "Reverse proxy" below).

### Reverse proxy

`docker compose up` bundles a Caddy reverse proxy by default (via the
auto-loaded `compose.override.yaml`), listening on `:80`/`:443` with
automatic HTTPS and routing `/api/*` to the backend and everything else to
the frontend, so both are served from a single origin.

If you already run your own reverse proxy in front of this stack, opt out of
the bundled one instead:

```
docker compose -f compose.yaml -f compose.external-proxy.yaml up
```

This drops the `caddy` service, removes the `backend`/`frontend` port
publishing, and attaches both to an externally-created `proxy_net` network
(create it first with `docker network create proxy_net`) for your own proxy
to join.

**Frontend dev outside Docker:** if you run the frontend with `npm run dev`
on the host instead of the `frontend` container, point Caddy at it by
setting `FRONTEND_UPSTREAM=host.docker.internal:3000` in `.env` before
starting compose, and access the app through Caddy (`https://localhost`,
or whatever `DOMAIN` you've configured) rather than hitting the dev
server's port directly. You'll also need to set `BACKEND_URL` (e.g.
`https://localhost`) for the `npm run dev` process itself -- e.g. via a
`.env.local` inside `KaleidoscopeFrontend/kaleidoscope/` -- since Compose's
environment injection only reaches the containerized frontend, not a
host-run one. This keeps frontend and backend on the same origin so cookies
and the session token round-trip correctly.

### Cookie security mode

The backend's `refresh_token` cookie is controlled by the `COOKIE_SECURITY_MODE`
environment variable on the `backend` service:

* `insecure`: no TLS, frontend and backend share a domain.
* `secure`: TLS in front of both (e.g. a reverse proxy), same domain.
* `cross-site`: frontend and backend on different domains, both served over HTTPS.

`compose.yaml` itself defaults this to `insecure`, but the bundled-Caddy
`compose.override.yaml` (the default `docker compose up` path) overrides it
to `secure`, since Caddy's automatic HTTPS guarantees TLS is present there.
If you opt out to `compose.external-proxy.yaml` instead, it stays at
`insecure` unless you set it yourself -- TLS presence depends on whatever
proxy you bring, which this project can't know in advance.

### Deploying your application to the cloud

First, build your image, e.g.: `docker build -t myapp .`.
If your cloud uses a different CPU architecture than your development
machine (e.g., you are on a Mac M1 and your cloud provider is amd64),
you'll want to build the image for that platform, e.g.:
`docker build --platform=linux/amd64 -t myapp .`.

Then, push it to your registry, e.g. `docker push myregistry.com/myapp`.

Consult Docker's [getting started](https://docs.docker.com/go/get-started-sharing/)
docs for more detail on building and pushing.

### References
* [Docker's Go guide](https://docs.docker.com/language/golang/)