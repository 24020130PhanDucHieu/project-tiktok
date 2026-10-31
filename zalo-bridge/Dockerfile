FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
COPY package*.json ./
RUN npm install --omit=dev
COPY src ./src
USER node
EXPOSE 5100
CMD ["node", "src/server.js"]
