# syntax=docker/dockerfile:1.7
FROM node:22-alpine AS build

WORKDIR /app

COPY web/package.json web/package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm npm install

COPY web/ .
RUN npm run build

FROM node:22-alpine

WORKDIR /app

COPY --from=build /app/.output .output

ENV HOST=0.0.0.0
ENV PORT=3000
ENV NITRO_PORT=3000

EXPOSE 3000

CMD ["node", ".output/server/index.mjs"]
