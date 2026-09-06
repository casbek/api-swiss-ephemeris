package swe

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
)

// ErrPoolClosed, kapatilmis bir havuza is gonderildiginde doner.
var ErrPoolClosed = errors.New("swe: havuz kapatildi")

// pool, Swiss Ephemeris cagrilarini kendine ait OS thread'leri uzerinde
// calistiran bir is havuzudur.
//
// Neden gerekli: Swiss Ephemeris global durum tutar. Linux/GCC uzerinde bu
// durum thread-local'dir (sweodef.h icindeki TLS tanimi), yani her OS
// thread'inin kendi swe_set_ephe_path cagrisina ve kendi dosya onbellegine
// ihtiyaci vardir. Windows'ta ise TLS devre disidir ve durum tum thread'ler
// arasinda paylasilir, dolayisiyla es zamanli cagrilar veri yarisi yaratir.
//
// Her iki durumu da tek cozumle karsilamak icin worker'lar runtime.LockOSThread
// ile sabit bir OS thread'ine baglanir ve o thread uzerinde bir kez
// ilklendirilir. Isler kanal uzerinden bu thread'lere yonlendirilir.
type pool struct {
	tasks chan *task
	quit  chan struct{}
	wg    sync.WaitGroup
	once  sync.Once
}

type task struct {
	fn   func()
	done chan struct{}
}

// newPool, ephePath dizinini kullanan ve workers adet OS thread'i ayiran
// bir havuz olusturur.
//
// workers icin varsayilan 1'dir ve cogu kurulum icin yeterlidir: tek bir
// natal harita hesabi 1 ms'nin altinda surer. Linux'ta TLS aktif oldugu icin
// deger guvenle artirilabilir; ancak her worker kendi ephemeris dosya
// onbellegini tuttugundan bellek kullanimi worker sayisiyla dogru orantili
// buyur. Windows'ta 1'den buyuk deger GUVENLI DEGILDIR.
func newPool(ephePath string, workers int) (*pool, error) {
	if workers < 1 {
		workers = 1
	}
	if runtime.GOOS == "windows" && workers > 1 {
		return nil, fmt.Errorf("swe: Windows uzerinde Swiss Ephemeris thread-safe degil, workers=1 olmali (verilen: %d)", workers)
	}

	p := &pool{
		tasks: make(chan *task),
		quit:  make(chan struct{}),
	}

	ready := make(chan struct{})
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.run(ephePath, ready)
	}
	// Tum worker'lar ephemeris yolunu ayarlayana kadar bekle.
	for i := 0; i < workers; i++ {
		<-ready
	}
	return p, nil
}

// run, tek bir worker'in dongusudur. Kendini bir OS thread'ine kilitler,
// Swiss Ephemeris'i o thread icin ilklendirir ve isleri sirayla calistirir.
func (p *pool) run(ephePath string, ready chan<- struct{}) {
	defer p.wg.Done()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	libswe.SetEphePath(ephePath)
	// Close, bu thread'in acik tuttugu ephemeris dosyalarini serbest birakir.
	defer libswe.Close()

	ready <- struct{}{}

	for {
		select {
		case t := <-p.tasks:
			t.fn()
			close(t.done)
		case <-p.quit:
			return
		}
	}
}

// do, fn fonksiyonunu bir Swiss Ephemeris thread'i uzerinde calistirir ve
// bitmesini bekler.
//
// ctx yalnizca is siraya alinirken beklemeyi kesebilir. Is bir kez worker'a
// verildikten sonra bitmesi beklenir; aksi halde fn'in cagiranin degiskenlerine
// yaptigi yazmalar veri yarisi olustururdu.
func (p *pool) do(ctx context.Context, fn func()) error {
	t := &task{fn: fn, done: make(chan struct{})}

	select {
	case p.tasks <- t:
	case <-ctx.Done():
		return ctx.Err()
	case <-p.quit:
		return ErrPoolClosed
	}

	<-t.done
	return nil
}

// close, worker'lari durdurur ve ephemeris dosyalarini kapatir.
func (p *pool) close() {
	p.once.Do(func() {
		close(p.quit)
		p.wg.Wait()
	})
}
