FROM node:20-alpine AS builder
WORKDIR /app

COPY web/package.json web/package-lock.json* web/pnpm-lock.yaml* web/yarn.lock* ./
RUN npm install

COPY web/ .
# В продакшене API идёт через Traefik по тому же домену — относительные пути /api
ENV VITE_API_PROXY_TARGET=
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
