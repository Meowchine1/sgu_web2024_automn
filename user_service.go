import (
	"encoding/json"
	"fmt"
	"log"
	"github.com/rabbitmq/amqp091-go"
	"microservices/config"
    "./config"
) 

func main() {
	// Подключение к RabbitMQ
	conn, err := amqp.Dial(config.RabbitMQURL)
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	// Подключение к базе данных (PostgreSQL)
	db, err := sql.Open("postgres", "user=postgres dbname=userservice sslmode=disable")
	failOnError(err, "Failed to connect to database")
	defer db.Close()

    // Объявление очереди для отправки уведомлений о создании заказа
    qCreateOrders, err := declareQueue(ch, config.order_create_queue_name)
	// Объявление очереди для получения уведомлений о доставке
	qDelivery, err :=declareQueue(ch, config.delivery_queue_name)
	 

	// Канал для завершения программы
	forever := make(chan struct{})

	// Горутина для прослушивания очереди "delivery"
	go func() {
		msgs, err := ch.Consume(
			qDelivery.Name, // очередь
			"",             // consumer
			true,           // auto-ack
			false,          // exclusive
			false,          // no-local
			false,          // no-wait
			nil,            // args
		)
		failOnError(err, "Failed to register a consumer for 'delivery' queue")

		// Горутина для обработки полученных сообщений
		for d := range msgs {
			log.Printf("Received delivery notification: %s", d.Body)

			// Вставка события в таблицу Outbox
			event := config.InboxEvent{
				Aggregate: "UserService",
				EventType: "OrderDelivered",
				Payload:   fmt.Sprintf("Order %s delivered", d.Body),
				CreatedAt: time.Now(),
			}
			err := config.insertInboxEvent(db, event)
			failOnError(err, "Failed to insert event into inbox")

			log.Printf("Order %s has been delivered. User notified.", d.Body)
		}
	}()

	// Горутина для отправки заказов раз в 5 секунд
	go func() { 
		for { 
			// Создание события заказа
			event := config.OutboxEvent{
				Aggregate: "UserService",
				EventType: "OrderCreated",
				Payload:   fmt.Sprintf("New order created" ),
				CreatedAt: time.Now(),
			}

			// Вставка события в таблицу Outbox
			err = config.insertOutboxEvent(db, event)
			failOnError(err, "Failed to insert event into outbox")

			// Отправка события о создании заказа в очередь RabbitMQ
			err = ch.Publish(
				"",               // exchange
				qCreateOrders.Name,  // routing key
				false,            // mandatory
				false,            // immediate
				amqp.Publishing{
					ContentType: "text/plain",
					Body:        []byte(event.Payload),
				},
			)
			failOnError(err, "Failed to publish a message to 'order_create__queue' queue")
			log.Printf(" [x] Order %s created and sent to 'order_create__queue' queue", orderIDStr)
 
			// Задержка 5 секунд
			time.Sleep(5 * time.Second)
		}
	}()

	log.Printf(" [*] Waiting for delivery notifications and creating orders. To exit press CTRL+C")

	// Ожидаем завершения программы
	<-forever
}

 