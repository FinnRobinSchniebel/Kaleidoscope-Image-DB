### Building and running your application

When you're ready, start your application by running:
`docker compose up --build`.

Your application will be available at http://localhost:3000.

### Reverse proxy

`docker compose up` bundles a Caddy reverse proxy by default (via the
auto-loaded `compose.override.yaml`), listening on `:80`/`:443` and routing
`/api/*` to the backend and everything else to the frontend, so both are
served from a single origin.

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
starting compose, and access the app through Caddy (`http://localhost/`)
rather than hitting the dev server's port directly. This keeps frontend and
backend on the same origin so the `refresh_token` cookie (SameSite=Lax under
the default `COOKIE_SECURITY_MODE=insecure`) is sent back on API requests.

### Cookie security mode

The backend's `refresh_token` cookie is controlled by the `COOKIE_SECURITY_MODE`
environment variable on the `backend` service:

* `insecure` (default): no TLS, frontend and backend share a domain.
* `secure`: TLS in front of both (e.g. a reverse proxy), same domain.
* `cross-site`: frontend and backend on different domains, both served over HTTPS.

Set this to `secure` or `cross-site` once you have HTTPS in front of the app;
leaving it at the default over plain HTTP is fine for local/self-hosted use
but exposes the refresh token to network interception.

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