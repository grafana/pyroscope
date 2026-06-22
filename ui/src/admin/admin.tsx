import ReactDOM from 'react-dom/client';
import { AdminApp } from './AdminApp';
import './styles.css';

const container = document.getElementById('root');
if (container) {
  const root = ReactDOM.createRoot(container);
  root.render(<AdminApp />);
}
