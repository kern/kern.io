FROM node:5.11
MAINTAINER Alex Kern <alex@pavlovml.com>

# install
RUN mkdir -p /kernio
WORKDIR /kernio
COPY package.json ./
RUN npm install
COPY . .

# build
ENV NODE_ENV=production PORT=80
RUN ./node_modules/.bin/stylus static -c -u nib --import nib css/app.styl > static/app.css && \
    rm -rf css && \
    npm prune

# run
EXPOSE 80
CMD [ "node", "index.js" ]
