ARG NODE_VERSION=22
ARG NGINX_VERSION=1.28-alpine

FROM node:${NODE_VERSION}-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM nginx:${NGINX_VERSION}

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=6 --start-period=5s \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/health/ready || exit 1
