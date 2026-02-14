FROM node:24.0.0 AS build

WORKDIR /app

COPY . .

RUN npm install --skip-confirmation &&  \
    npm install -g @angular/cli --skip-confirmation &&  \
    npm install -g gzipper --skip-confirmation &&  \
    ng build --aot &&  \
    gzipper compress ./dist



FROM nginx:alpine

RUN apk update  && apk upgrade && rm /etc/nginx/conf.d/default.conf

COPY build/default.conf /etc/nginx/conf.d/default.conf

COPY --from=build /app/dist/b0k3ts/browser /usr/share/nginx/html

USER nginx
