import React from 'react';
import ReactDOM from 'react-dom/client';
import './jquery-import';
import { SingleView } from './pages/SingleView';

const root = ReactDOM.createRoot(document.getElementById('reactRoot'));
root.render(<SingleView />);
