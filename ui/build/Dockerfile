FROM node:24.0.0 AS build

WORKDIR /app

COPY . .

RUN npm install --skip-confirmation

RUN npm install -g @angular/cli --skip-confirmation

RUN npm install -g gzipper --skip-confirmation

RUN ng build --aot && gzipper compress ./dist

FROM nginx:alpine

RUN apk update  && apk upgrade

RUN rm /etc/nginx/conf.d/default.conf

COPY build/default.conf /etc/nginx/conf.d/default.conf

COPY --from=build /app/dist/b0k3ts/browser /usr/share/nginx/html

USER nginx
