Rails.application.routes.draw do
  # Define your application routes per the DSL in https://guides.rubyonrails.org/routing.html

  # Defines the root path route ("/")
  # root "articles#index"
  get "/bike" => "bike#index"
  get "/scooter" => "scooter#index"
  get "/car" => "car#index"
end
