import Navbar from './components/home/Navbar'
import HeroSection from './components/home/HeroSection'
import FeaturesSection from './components/home/FeaturesSection'
import QuickStartSection from './components/home/QuickStartSection'
import Footer from './components/home/Footer'

export default function Home() {
  return (
    <div className="min-h-screen">
      <Navbar />
      <HeroSection />
      <FeaturesSection />
      <QuickStartSection />
      <Footer />
    </div>
  )
}
