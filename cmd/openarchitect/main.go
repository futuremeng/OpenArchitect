package main

import (
	"context"
	"log"
	"path/filepath"

	"github.com/egoist/mygo"

	"github.com/futuremeng/OpenArchitect/internal/core/store"
	appui "github.com/futuremeng/OpenArchitect/internal/ui"
)

// main 包和 internal/ui 都叫 ui，所以自己的那个必须起别名（Task 16 那条
// "本包自己叫 ui 又 import mygo/ui 合法"的规则只在包内成立，导入方两个都要）。
func main() {
	// 名字决定 PathUserData = os.UserConfigDir() + App.Name()（mygo/paths.go），
	// 也就决定库目录，必须和 cmd/oac 的 defaultDBPath 对齐（spec §5.2 之后那段）。
	mygo.App.SetName("OpenArchitect")
	mygo.App.WhenReady(func() {
		// Path 要 app ready 之后才有意义，所以开库只能待在这个回调里。
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatalf("拿不到用户数据目录: %v", err)
		}
		db, err := store.Open(context.Background(), filepath.Join(dir, "openarchitect.db"))
		if err != nil {
			log.Fatalf("打开数据库 %s: %v", filepath.Join(dir, "openarchitect.db"), err)
		}
		a := appui.NewApp(db)
		if err := a.Reload(context.Background()); err != nil {
			// 空库、旧库都不该拦住窗口：页面自己有"还没扫描过"的引导态。
			log.Printf("首次载入: %v", err)
		}
		s := a.NewShell()
		a.SetPost(s.Window().Update)
		a.SetOpenPath(mygo.Shell.OpenPath)
		s.Window().OnClosed(func() {
			// spec §10：关窗即取消在跑的扫描，然后收库、退出。
			a.CancelScan()
			if err := db.Close(); err != nil {
				log.Printf("关闭数据库: %v", err)
			}
			mygo.App.Quit()
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

