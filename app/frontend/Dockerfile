# Build stage
FROM node:18-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* ./

# Ставим всё (включая dev)
RUN npm ci

# Явно ставим tsc глобально, он появится в $PATH
RUN npm install -g typescript

COPY . .
RUN npm run build

# Runtime stage
FROM node:18-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=80
COPY --from=build /app/dist ./dist
COPY --from=build /app/static ./static
COPY package.json package-lock.json* ./
RUN npm ci --omit=dev
EXPOSE 80
CMD ["node", "dist/server.js"]