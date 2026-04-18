# Build stage
FROM node:18-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install
COPY . .
RUN npm run build

# Runtime stage
FROM node:18-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=80
COPY --from=build /app/dist ./dist
COPY --from=build /app/static ./static
COPY package.json ./package.json
RUN npm install --production
EXPOSE 80
CMD ["node", "dist/server.js"]
