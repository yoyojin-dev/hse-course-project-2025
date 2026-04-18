# Build stage
FROM node:18-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* ./
# Install all dependencies (including dev) so that tsc and build tools are available
RUN npm ci
COPY . .
RUN npm run build

# Runtime stage
FROM node:18-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=80
COPY --from=build /app/dist ./dist
COPY --from=build /app/static ./static
# Copy package.json and package-lock.json if present so `npm ci` can run in runtime stage
COPY package.json package-lock.json* ./
# Install only production dependencies in runtime image
RUN npm ci --omit=dev
EXPOSE 80
CMD ["node", "dist/server.js"]
