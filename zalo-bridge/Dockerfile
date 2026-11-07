FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
# listen on every interface of the container so the API container can reach it; compose publishes no port for it
ENV HOST=0.0.0.0
COPY package*.json ./
RUN npm install --omit=dev
COPY src ./src
USER node
EXPOSE 5100
CMD ["node", "src/server.js"]
