Rails.application.routes.draw do
  # Define your application routes per the DSL in https://guides.rubyonrails.org/routing.html

  # Defines the root path route ("/")
  # root "articles#index"
  get '/car', to: 'car/car#show'
  get '/bike', to: 'bike/bike#show'
  get '/scooter', to: 'scooter/scooter#show'
end
