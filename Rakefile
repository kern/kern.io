task :server do
  sh "rackup"
end

task :sass do
  sh "sass --watch css/app.scss:public/app.css"
end

multitask :default => [:server, :sass]
