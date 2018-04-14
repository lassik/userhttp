#! /usr/local/bin/ruby

require 'rack'

Rack::Server.start(
  :app => lambda do |e|
    [200, {'Content-Type' => 'text/html'}, ['<h1>Hello, world!</h1>']]
  end,
  :server => 'cgi',
)
