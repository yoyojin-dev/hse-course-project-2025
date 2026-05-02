import React from 'react';
import { Route, Routes } from 'react-router-dom';
import LandingPage from './pages/LandingPage';
import StartPage from './pages/StartPage';
import JoiningPage from './pages/JoiningPage';
import CreatedPage from './pages/CreatedPage';
import GamePage from './pages/GamePage';

const App: React.FC = () => {
  return (
    <Routes>
      <Route path="/" element={<LandingPage />} />
      <Route path="/start" element={<StartPage />} />
      <Route path="/joining/:gamecode" element={<JoiningPage />} />
      <Route path="/created/:gamecode" element={<CreatedPage />} />
      <Route path="/game/:gamecode" element={<GamePage />} />
    </Routes>
  );
};

export default App;
