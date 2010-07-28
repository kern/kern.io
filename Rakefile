require 'bundler'
Bundler.setup

require 'toto'

@config = Toto::Config::Defaults

task :default => :new

desc "Create a new article."
task :new do
  title = ask('Title: ')
  subtitle = ask('Subtitle: ')
  slug = title.empty?? nil : title.strip.slugize
  
  article = {'title' => title, 'subtitle' => subtitle}.to_yaml
  article << "\n"
  article << "Once upon a time...\n\n"
  
  path = "#{Toto::Paths[:articles]}/#{Time.now.strftime("%Y-%m-%d")}#{'-' + slug if slug}.#{@config[:ext]}"
  
  unless File.exist? path
    File.open(path, "w") do |file|
      file.write article
    end
    toto "an article was created for you at #{path}."
  else
    toto "I can't create the article, #{path} already exists."
  end
end

desc "Publish my blog."
task :publish do
  toto "publishing your article(s)..."
  `git push heroku master`
end

def toto msg
  puts "\n  toto ~ #{msg}\n\n"
end

def ask message
  print message
  STDIN.gets.chomp
end

desc 'Watch sass files.'
task :sass do
  `bundle exec sass --update public/sass:public/css`
end