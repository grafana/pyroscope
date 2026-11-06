Rails.application.routes.draw do
  get "/fast" => "work#fast"
  get "/slow" => "work#slow"
end
