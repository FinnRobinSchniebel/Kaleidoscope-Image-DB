### Building and running your application

When you're ready, start your application by running:
`docker compose up --build`.

Your application will be available at http://localhost:3000.

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