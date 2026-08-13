FROM node:22-alpine AS builder
WORKDIR /app
ARG API_URL=http://localhost:8080
ARG SITE_URL=http://localhost:3000
ARG NEXT_PUBLIC_GIPHY_KEY=
ENV API_URL=$API_URL
ENV SITE_URL=$SITE_URL
ENV NEXT_PUBLIC_GIPHY_KEY=$NEXT_PUBLIC_GIPHY_KEY
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production

# Create the runtime user BEFORE copying files so we can chown
# during COPY. Earlier this image created the user after COPY and
# only chowned .next/cache, which left .next/server/app/ owned by
# root. Next.js writes ISR / prerender data to .next/server/app/
# (NOT .next/cache) — every write failed with EACCES, breaking
# sitemap regeneration, prerender of `/`, prerender of community
# pages, and crucially the `/post/[id]` slugless-redirect route
# which falls back to the 404 page when its underlying revalidate
# call fails. Net effect: Google saw soft-404s on every slugless
# share link, dedup'd them against the home page, and stopped
# indexing the site.
RUN addgroup -S nodejs && adduser -S nextjs -G nodejs
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public ./public
RUN mkdir -p .next/cache && chown -R nextjs:nodejs .next
USER nextjs
EXPOSE 3000
CMD ["node", "server.js"]
