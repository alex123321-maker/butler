FROM mcr.microsoft.com/playwright:v1.52.0-jammy
RUN useradd -m -u 10001 butler
WORKDIR /app/apps/tool-browser
COPY apps/tool-browser/package.json apps/tool-browser/package-lock.json ./
RUN npm ci --omit=dev --ignore-scripts
USER butler
