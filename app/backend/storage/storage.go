// Тут интерфейс памяти со всеми нужными методами

package storage

// Storage - основной интерфейс памяти. Объединяет все подинтерфейсы
type Storage interface {
	UserStorage
	GamesStorage
}

type UserStorage interface {
	// CreateUser сохраняет нового пользователя и возвращает сгенерированный ID
	CreateUser(user interface{}) (id string, err error)

	// GetUser возвращает пользователя по ID, или ошибку, если не найден
	GetUser(id string) (user interface{}, err error)

	// GetUserByName возвращает пользователя по имени/логину
	GetUserByName(name string) (user interface{}, err error)

	// UpdateUser обновляет данные пользователя по ID
	UpdateUser(id string, user interface{}) error

	// DeleteUser удаляет пользователя по ID
	DeleteUser(id string) error
}

// GamesStorage — базовые операции с играми
type GamesStorage interface {
	// CreateGame создаёт новую игру и возвращает её ID
	CreateGame(game interface{}) (id string, err error)

	// GetGame возвращает игру по ID
	GetGame(id string) (game interface{}, err error)

	// ListGames возвращает список игр (пагинация может быть добавлена позже)
	ListGames() ([]interface{}, error)

	// UpdateGame обновляет игру по ID
	UpdateGame(id string, game interface{}) error

	// DeleteGame удаляет игру по ID
	DeleteGame(id string) error

	// ValidateGameID проверяет, что ID игры есть в базе
	ValidateGameID(id string) bool

	// JoinGame добавляет пользователя в игру; возвращает ошибка или nil
	JoinGame(gameID string, userID string) error

	// LeaveGame удаляет пользователя из игры
	LeaveGame(gameID string, userID string) error
}
